package github

import (
	"cmp"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The rate-limit resources gh-tui sends requests against, as GitHub names
// them. Each has a quota of its own, so running out of one, such as code
// search, leaves the others be. GitHub has more, which are kept under the
// names its responses give them.
const (
	resourceCore       = "core"
	resourceSearch     = "search"
	resourceCodeSearch = "code_search"
	resourceGraphQL    = "graphql"
)

// budget keeps track of the rate limits of the account the client acts
// as: for each resource, what GitHub last reported of it, and what the
// requests sent since and not answered yet will take of it. It is safe for
// concurrent use.
type budget struct {
	// restRoot and graphqlPath are the paths of the REST root, such as /
	// or /api/v3/, and of the GraphQL endpoint.
	restRoot    string
	graphqlPath string
	now         func() time.Time

	mu     sync.Mutex
	quotas map[string]*quota
	// pending are the requests sent and not answered yet, by the order
	// they were counted in.
	pending map[uint64]*reservation
	seq     uint64
	// opCost is what the last query of each GraphQL operation cost.
	opCost map[string]int
}

// quota is one resource, as the answers of GitHub report it.
type quota struct {
	limit, remaining int
	// asOf is the reservation whose answer reported remaining, so that
	// a late answer that GitHub counted earlier doesn't undo it.
	asOf uint64
	// reset is when the window refills, in GitHub's time, as reported.
	reset  time.Time
	seenAt time.Time
}

// reservation is a request counted against a resource before it is sent.
type reservation struct {
	seq      uint64
	resource string
	cost     int
}

// quotaStatus is what the budget knows of one resource at a moment.
type quotaStatus struct {
	// reported is what GitHub reported last.
	reported RateLimit
	// est is what is left once the requests in flight are answered, as
	// far as the budget can tell. It may be below zero.
	est    int
	seenAt time.Time
}

func newBudget(restRoot, graphqlPath string) *budget {
	return &budget{
		restRoot:    restRoot,
		graphqlPath: graphqlPath,
		now:         time.Now,
		quotas:      make(map[string]*quota),
		pending:     make(map[uint64]*reservation),
		opCost:      make(map[string]int),
	}
}

// classify returns the resource that req counts against, or "" if it
// counts against none, as GET /rate_limit doesn't.
func (b *budget) classify(req *http.Request) string {
	path := req.URL.EscapedPath()
	if path == b.graphqlPath {
		return resourceGraphQL
	}
	rest, ok := strings.CutPrefix(path, b.restRoot)
	switch {
	case !ok, rest == "rate_limit":
		return ""
	case rest == "search/code":
		return resourceCodeSearch
	case strings.HasPrefix(rest, "search/"):
		return resourceSearch
	}
	return resourceCore
}

// reserve counts req against its resource, before it is sent, and returns
// the reservation that observe or forget settles. A request costs 1, and
// a GraphQL query what its operation cost last time, since a query's cost
// depends on its shape more than on its variables. A mutation costs 1 of
// the primary limit. A request to a host outside the API counts against
// nothing and has no reservation.
func (b *budget) reserve(req *http.Request) *reservation {
	c, _ := req.Context().Value(callKey{}).(*call)
	if c != nil && c.external {
		return nil
	}
	resource := b.classify(req)
	cost := 0
	if resource != "" {
		cost = 1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if resource == resourceGraphQL && c != nil && c.query {
		cost = max(b.opCost[c.op], cost)
	}
	b.seq++
	r := &reservation{seq: b.seq, resource: resource, cost: cost}
	b.pending[r.seq] = r
	return r
}

// learnCost records what a query of the GraphQL operation op cost, as the
// rateLimit field of its data reported.
func (b *budget) learnCost(op string, cost int) {
	b.mu.Lock()
	b.opCost[op] = cost
	b.mu.Unlock()
}

// forget settles r, whose request got no answer.
func (b *budget) forget(r *reservation) {
	b.mu.Lock()
	delete(b.pending, r.seq)
	b.mu.Unlock()
}

// observe settles r with the headers h of its answer. The rate limit they
// report is counted in the resource they name, or in the resource of r if
// they name none. Answers may arrive in another order than their requests
// were sent, so within a window the lowest remaining wins, and one of an
// earlier window is ignored.
func (b *budget) observe(r *reservation, h http.Header) {
	now := b.now()
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.pending, r.seq)
	rl, ok := parseRateLimit(h)
	resource := cmp.Or(rl.Resource, r.resource)
	if !ok || resource == "" {
		return
	}
	q := b.quotas[resource]
	switch {
	case q == nil || rl.Reset.After(q.reset):
		q = &quota{reset: rl.Reset}
		b.quotas[resource] = q
	case rl.Reset.Before(q.reset):
		return
	case rl.Remaining > q.remaining || rl.Remaining == q.remaining && r.seq < q.asOf:
		// An answer that GitHub counted before the one that set remaining.
		q.seenAt = now
		return
	}
	q.limit, q.remaining, q.asOf, q.seenAt = rl.Limit, rl.Remaining, r.seq, now
}

// status returns what the budget knows of resource, or false if GitHub
// never reported it, as a GitHub Enterprise Server with rate limits turned
// off doesn't.
func (b *budget) status(resource string) (quotaStatus, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	q := b.quotas[resource]
	if q == nil {
		return quotaStatus{}, false
	}
	return quotaStatus{
		reported: RateLimit{Limit: q.limit, Remaining: q.remaining, Reset: q.reset, Resource: resource},
		est:      b.est(resource, q),
		seenAt:   q.seenAt,
	}, true
}

// est returns what is left of q, the quota of resource, once the requests
// in flight are answered. GitHub may count them in another order than
// they were reserved, and some may still wait for a slot, so none of them
// is taken to be in q.remaining yet: the estimate may be short by what is
// on the wire, never over. b.mu must be held.
func (b *budget) est(resource string, q *quota) int {
	n := q.remaining
	for _, r := range b.pending {
		if r.resource == resource {
			n -= r.cost
		}
	}
	return n
}

// rateTransport counts each attempt at a request in the budget before it
// is sent, and has the budget learn from its answer. It sits below retry,
// so that the attempts retry discards are counted too, and above the
// limit, so that a request waiting for a slot is counted already.
type rateTransport struct {
	base   http.RoundTripper
	budget *budget
}

// RoundTrip sends req and observes its answer.
func (t *rateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := t.budget.reserve(req)
	if r == nil {
		return t.base.RoundTrip(req)
	}
	// A reservation left behind by a panic would take from the
	// estimate for good.
	settled := false
	defer func() {
		if !settled {
			t.budget.forget(r)
		}
	}()
	resp, err := t.base.RoundTrip(req)
	settled = true
	if err != nil {
		t.budget.forget(r)
		return nil, err
	}
	t.budget.observe(r, resp.Header)
	return resp, nil
}
