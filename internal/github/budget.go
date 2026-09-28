package github

import (
	"cmp"
	"context"
	"net/http"
	"slices"
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

const (
	// minGuard is how long after its reset a spent resource is used
	// again. The reset is in whole seconds, and whether GitHub rounds or
	// truncates it isn't documented, so up to a second of what looks like
	// the new window may still be the old one.
	minGuard = time.Second
	// maxGuard bounds the guard, which doubles each time a request
	// reserved after it still finds the old window.
	maxGuard = 8 * time.Second
	// maxWindow is how far away a reset may be. The longest window is an
	// hour, so a reset further away is malformed, and would stop requests
	// for no reason.
	maxWindow = 61 * time.Minute
	// skewSamples is how many offsets of GitHub's clock the skew is the
	// median of.
	skewSamples = 5
	// maxLearned bounds the routes whose resource the budget learned.
	// Past it, it forgets them and learns again.
	maxLearned = 64
)

// budget keeps track of the rate limits of the account the client acts
// as: for each resource, what GitHub last reported of it, and what the
// requests sent since and not answered yet will take of it. GitHub reports
// its resets in its own clock, so the budget learns how far that is from
// the local one, and a spent resource is released a guard after its
// reset in local time. Its gate holds back the requests that the limits
// would refuse (admit). It is safe for concurrent use.
type budget struct {
	// host is the host of the API, with its port if it has one, and
	// restRoot and graphqlPath are the paths of the REST root, such as /
	// or /api/v3/, and of the GraphQL endpoint.
	host        string
	restRoot    string
	graphqlPath string
	now         func() time.Time

	mu     sync.Mutex
	skew   clockSkew
	quotas map[string]*quota
	// pending are the requests sent and not answered yet, by the order
	// they were counted in.
	pending map[uint64]*reservation
	seq     uint64
	// opCost is what the last query of each shape (call.shape) cost.
	opCost map[string]int
	// learned is the resource that answers named for a REST route, by
	// method and route, where it isn't core, as for a dependency
	// snapshot, which counts against dependency_snapshots.
	learned map[string]string
	// answered is when GitHub last answered, and failed when a request
	// last got no answer, for the status of the connection. rejected is
	// when an answer last rejected the token, and accepted when one last
	// didn't.
	answered, failed   time.Time
	rejected, accepted time.Time

	// logs are what was logged while b.mu was held, which unlock logs
	// once it is released.
	logs []func()

	// notifier tells of the changes that a status bar shows, or is nil.
	// Each method that may make one calls changed once b.mu is released.
	notifier *rateNotifier
	// gate holds requests back while their limits are on.
	gate gate
}

// quota is one resource, as the answers of GitHub report it.
type quota struct {
	limit, remaining int
	// asOf is the reservation whose answer reported remaining, so that
	// a late answer that GitHub counted earlier doesn't undo it.
	asOf uint64
	// reset is when the window refills, in GitHub's time, as reported.
	reset time.Time
	// release is when the resource may be used again, in local time, or
	// zero if it isn't spent.
	release time.Time
	// guard is how long after the reset the release is.
	guard  time.Duration
	seenAt time.Time
}

// reservation is a request counted against a resource before it is sent.
type reservation struct {
	seq      uint64
	resource string
	// route is the method and route of a REST request that the path
	// says counts against core, so that its answer can say otherwise.
	route string
	cost  int
	// at is when the request was reserved, which is before it waited
	// for a slot, so it may have gone out later.
	at time.Time
	// sent is set once the request has its slot and goes out, and
	// recalled once a limit that came before then stopped it; cancel
	// ends its attempt, to recall it. b.mu guards them.
	sent, recalled bool
	cancel         context.CancelCauseFunc
}

// quotaStatus is what the budget knows of one resource at a moment.
type quotaStatus struct {
	// reported is what GitHub reported last.
	reported RateLimit
	// est is what is left once the requests in flight are answered, as
	// far as the budget can tell. It may be below zero.
	est int
	// release is when a spent resource may be used again, in local
	// time, or zero if it isn't spent.
	release time.Time
	seenAt  time.Time
}

func newBudget(host, restRoot, graphqlPath string) *budget {
	return &budget{
		host:        host,
		restRoot:    restRoot,
		graphqlPath: graphqlPath,
		now:         time.Now,
		quotas:      make(map[string]*quota),
		pending:     make(map[uint64]*reservation),
		opCost:      make(map[string]int),
		learned:     make(map[string]string),
		gate:        newGate(),
	}
}

// classify returns the resource that req counts against, or "" if it
// counts against none, as GET /rate_limit doesn't, nor a request to
// another host, such as a redirect to where a download is stored.
func (b *budget) classify(req *http.Request) string {
	if req.URL.Host != b.host {
		return ""
	}
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

// reserve counts a request of resource that costs cost against it before
// it is sent, and returns the reservation that observe or forget settles.
// route is the request's method and route if it is a REST request that
// its path says counts against core. b.mu must be held.
func (b *budget) reserve(resource, route string, cost int, now time.Time) *reservation {
	b.seq++
	r := &reservation{seq: b.seq, resource: resource, route: route, cost: cost, at: now}
	b.pending[r.seq] = r
	return r
}

// route returns the method and route of req, a REST request, such as
// POST /repos/{owner}/{repo}/dependency-graph/snapshots.
func (b *budget) route(req *http.Request) string {
	route, _ := restRoute(strings.TrimPrefix(req.URL.EscapedPath(), b.restRoot))
	return req.Method + " " + route
}

// learnResource records that the answer to a request of route counted
// against resource. b.mu must be held.
func (b *budget) learnResource(route, resource string) {
	switch {
	case route == "", resource == "":
	case resource == resourceCore:
		delete(b.learned, route)
	case b.learned[route] != resource:
		if len(b.learned) >= maxLearned {
			clear(b.learned)
		}
		b.learned[route] = resource
	}
}

// learnCost records what a GraphQL query of shape cost, as the rateLimit
// field of its data reported. A query costs what a query of its shape cost
// last time, since its cost depends on its shape more than on its
// variables.
func (b *budget) learnCost(shape string, cost int) {
	b.mu.Lock()
	b.opCost[shape] = cost
	b.mu.Unlock()
}

// forget settles r, whose request got no answer. failed says that the
// request failed, rather than was canceled or never sent.
func (b *budget) forget(r *reservation, failed bool) {
	now := b.now()
	defer b.changed()
	b.mu.Lock()
	defer b.unlock()
	delete(b.pending, r.seq)
	if failed {
		b.failed = now
	}
	b.settle(r, false)
}

// log has f log a record once b.mu is released, so that no handler of
// the log is called with it held. b.mu must be held.
func (b *budget) log(f func()) {
	b.logs = append(b.logs, f)
}

// unlock releases b.mu, and then logs what was logged while it was held.
func (b *budget) unlock() {
	logs := b.logs
	b.logs = nil
	b.mu.Unlock()
	for _, f := range logs {
		f()
	}
}

// contact returns when GitHub last answered a request, and when a request
// last got no answer, each zero before the first.
func (b *budget) contact() (answered, failed time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.answered, b.failed
}

// observe settles r with the status and the headers h of its answer, and
// returns the new guard of its resource if the answer shows that the guard
// was too short. The rate limit they report is counted in the resource
// they name, or in the resource of r if they name none.
func (b *budget) observe(r *reservation, status int, h http.Header) (guard time.Duration) {
	now := b.now()
	defer b.changed()
	b.mu.Lock()
	defer b.unlock()
	defer b.settle(r, true)
	delete(b.pending, r.seq)
	// Only GitHub's answers count, not a page of a captive portal or a
	// proxy on the way, whose clock may be another.
	if h.Get("X-GitHub-Request-Id") != "" {
		b.answered = now
		// A 401 is GitHub rejecting the token, whatever was asked; a
		// missing scope is a 403 or a 404.
		if status == http.StatusUnauthorized {
			b.rejected = now
		} else {
			b.accepted = now
		}
		if date, err := http.ParseTime(h.Get("Date")); err == nil {
			b.skew.add(date.Sub(now))
		}
	}
	rl, ok := parseRateLimit(h)
	resource := cmp.Or(rl.Resource, r.resource)
	if !ok || resource == "" {
		return 0
	}
	b.learnResource(r.route, rl.Resource)
	return b.report(resource, rl, r.seq, r.at, now)
}

// report counts rl in the quota of resource, as the answer to a request
// reserved as seq at at reported it. It is the one place that what GitHub
// says of a quota is taken in, and answers may arrive in another order
// than their requests were sent, so the reset never moves back and, within
// a window, remaining never goes up: one of an earlier window is ignored,
// as is a reset too far away to be real, unless the window kept is one
// such, and within a window the lowest remaining wins. A later reset
// starts a new window, whatever is left in it. So does a higher limit in
// the same window, as when GitHub raises the account's; a lower one is
// the limit before the raise, from a request GitHub counted before it, so
// it is ignored, and the window never goes back to it. A lower limit that
// GitHub set for real takes effect at the next reset, and meanwhile a
// refusal still holds the resource (refused). A new window starts with the
// shortest guard; a request reserved after the release that still finds
// the old window spent doubles it, which report returns. b.mu must be
// held.
func (b *budget) report(resource string, rl RateLimit, seq uint64, at, now time.Time) (guard time.Duration) {
	far := b.far(rl.Reset, now)
	q := b.quotas[resource]
	switch {
	case q == nil || !far && (rl.Reset.After(q.reset) || b.far(q.reset, now)):
		q = &quota{reset: rl.Reset, guard: minGuard}
		b.quotas[resource] = q
	case !rl.Reset.Equal(q.reset):
		return 0
	case rl.Limit > q.limit:
		// GitHub raised the limit, which starts the window over, with
		// the shortest guard and nothing spent.
		*q = quota{reset: q.reset, guard: minGuard}
	case rl.Limit < q.limit:
		// The limit before a raise, from a request GitHub counted
		// before it, whatever order they were reserved in.
		q.seenAt = now
		return 0
	case rl.Remaining > q.remaining || rl.Remaining == q.remaining && seq < q.asOf:
		// An answer that GitHub counted before the one that set remaining.
		q.seenAt = now
		return 0
	}
	q.limit, q.remaining, q.asOf, q.seenAt = rl.Limit, rl.Remaining, seq, now
	if q.remaining > 0 {
		return 0
	}
	if !q.release.IsZero() && !at.Before(q.release) {
		q.guard = min(2*q.guard, maxGuard)
		q.release = time.Time{}
		guard = q.guard
	}
	if q.release.IsZero() {
		q.release = b.release(q.reset, q.guard, now)
	}
	return guard
}

// refused records that GitHub refused a request because the quota of rl
// is spent, as a 403 or a GraphQL RATE_LIMITED error says, and returns
// when to send one again: the release of the resource rl names, or else
// of resource. A query may be refused with quota left, when it costs more
// than is left, and then too the resource is spent until its reset, and
// the requests of it not sent yet are recalled. It returns false if the
// reset is too far away to be real.
func (b *budget) refused(resource string, rl RateLimit) (time.Time, bool) {
	resource = cmp.Or(rl.Resource, resource)
	now := b.now()
	defer b.changed()
	b.mu.Lock()
	defer b.mu.Unlock()
	q := b.quotas[resource]
	if q == nil || !rl.Reset.Equal(q.reset) {
		// A window the budget doesn't keep, such as one that passed
		// before this late answer came.
		at := b.release(rl.Reset, minGuard, now)
		return at, !at.IsZero()
	}
	if q.release.IsZero() {
		q.release = b.release(q.reset, q.guard, now)
		b.recall(now)
	}
	return q.release, !q.release.IsZero()
}

// release returns when a resource spent until reset, in GitHub's clock, may
// be used again: guard after the reset in local time, or after now if
// that has passed. It is zero if the reset is too far away to be real.
// b.mu must be held.
func (b *budget) release(reset time.Time, guard time.Duration, now time.Time) time.Time {
	if b.far(reset, now) {
		return time.Time{}
	}
	at := b.local(reset)
	if at.Before(now) {
		at = now
	}
	return at.Add(guard)
}

// far reports whether reset, in GitHub's clock, is too far away to be
// real. b.mu must be held.
func (b *budget) far(reset, now time.Time) bool {
	return b.local(reset).After(now.Add(maxWindow))
}

// costsMore reports whether a GraphQL query of shape costs more than
// remaining, as far as what it cost last time says.
func (b *budget) costsMore(shape string, remaining int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.opCost[shape] > remaining
}

// local returns t, a time in GitHub's clock, in the local one. b.mu must
// be held.
func (b *budget) local(t time.Time) time.Time {
	return t.Add(-b.skew.offset())
}

// localTime is local for a caller that doesn't hold b.mu.
func (b *budget) localTime(t time.Time) time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.local(t)
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
		release:  q.release,
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

// clockSkew estimates how far GitHub's clock is ahead of the local one,
// from the Date of its answers less the time they arrived. It is the
// median of the last few, so that one answer held up on its way doesn't
// move it. Date is in whole seconds, so an offset may be up to a second
// short, which only makes a release later.
type clockSkew struct {
	offsets [skewSamples]time.Duration
	n, next int
}

func (s *clockSkew) add(d time.Duration) {
	s.offsets[s.next] = d
	s.next = (s.next + 1) % skewSamples
	s.n = min(s.n+1, skewSamples)
}

// offset returns the median offset, the lower one of an even count, or 0
// before any.
func (s *clockSkew) offset() time.Duration {
	if s.n == 0 {
		return 0
	}
	sorted := slices.Clone(s.offsets[:s.n])
	slices.Sort(sorted)
	return sorted[(s.n-1)/2]
}
