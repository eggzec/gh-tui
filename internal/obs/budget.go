package obs

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// Reads ahead, such as the details of the first rows of a list, are
// guesses, so they may spend only a share of each hourly quota: the share
// SetPrefetchBudget sets of the GraphQL points, and of the REST core
// requests. Each counts against the limit GitHub last reported, or
// fallbackLimit before it reported one. Once they spent either budget, no
// more are read ahead until that quota refills, or for
// fallbackPrefetchWindow if GitHub reported none; then it starts over.
// What the user asks for is never held back. GraphQL queries whose cost is
// unknown count as one point, the least a query costs; REST requests that
// GitHub answered 304 Not Modified are free against the quota, so they
// count nothing.
const (
	fallbackLimit          = 5000
	fallbackPrefetchWindow = time.Hour
)

// prefetchShare is the percent of each quota the reads ahead may spend,
// which SetPrefetchBudget sets. Until it does, the first read ahead spends
// the budget.
var prefetchShare atomic.Int64

// SetPrefetchBudget sets the percent of each hourly quota, GraphQL points
// and REST core requests alike, that the reads ahead may spend. It applies
// from the next read ahead, or check of PrefetchSpent, on.
func SetPrefetchBudget(percent int) {
	prefetchShare.Store(int64(percent))
}

// The quotas the reads ahead have a budget of, by their index in
// budgetQuotas.
const (
	budgetGraphQL = iota
	budgetCore
)

var budgetQuotas = [...]quotaKey{budgetGraphQL: {GraphQL, "graphql"}, budgetCore: {REST, "core"}}

// prefetchBudget is what the reads ahead spent of their budget of one
// quota, in the window of the quota they spent it in.
type prefetchBudget struct {
	used int64
	// spent is set once used reached the budget.
	spent bool
	// renew is when the window that used counts in ends, and the budget
	// starts over, spent or not.
	renew time.Time
}

// ChargeGraphQL counts a GraphQL query sent with ctx that cost cost points,
// or 0 if unknown, against the default stats' prefetch budget, if ctx is a
// read ahead (ForPrefetch).
func ChargeGraphQL(ctx context.Context, cost int) {
	if IsPrefetch(ctx) {
		Default().chargePrefetch(ctx, budgetGraphQL, int64(max(cost, 1)))
	}
}

// ChargeREST counts a REST request sent with ctx against the default
// stats' prefetch budget, if ctx is a read ahead (ForPrefetch) and the
// request counted against the core quota: resource is the quota GitHub
// named, and notModified whether it answered 304, which costs nothing.
func ChargeREST(ctx context.Context, resource string, notModified bool) {
	if IsPrefetch(ctx) && resource == budgetQuotas[budgetCore].resource && !notModified {
		Default().chargePrefetch(ctx, budgetCore, 1)
	}
}

// PrefetchSpent reports whether the reads ahead spent their budget of any
// quota, in the default stats, until that quota refills.
func PrefetchSpent() bool {
	s := Default()
	spent := false
	for i := range budgetQuotas {
		spent = s.prefetchUse(context.Background(), i, time.Now()).Spent || spent
	}
	return spent
}

// prefetchUse returns what the reads ahead spent of their budget of quota
// i, with the budget renewed if its window ended and marked spent if a
// lower share spent it, which it logs.
func (s *Stats) prefetchUse(ctx context.Context, i int, now time.Time) BudgetUse {
	budget, reset := s.prefetchLimit(i)
	s.budgetMu.Lock()
	over := s.checkPrefetch(i, budget, reset, now)
	b := s.budget[i]
	s.budgetMu.Unlock()
	if over {
		logSpent(ctx, i, b, budget)
	}
	use := BudgetUse{Used: b.used, Budget: budget, Spent: b.spent}
	if b.spent {
		use.Until = &b.renew
	}
	return use
}

// chargePrefetch counts cost against the budget of quota i.
func (s *Stats) chargePrefetch(ctx context.Context, i int, cost int64) {
	budget, reset := s.prefetchLimit(i)
	now := time.Now()
	s.budgetMu.Lock()
	s.renewPrefetch(i, now)
	if s.budget[i].used == 0 {
		s.budget[i].renew = windowEnd(reset, now)
	}
	s.budget[i].used += cost
	over := s.checkPrefetch(i, budget, reset, now)
	b := s.budget[i]
	s.budgetMu.Unlock()
	if over {
		logSpent(ctx, i, b, budget)
	}
}

// logSpent logs that the reads ahead spent b, their budget of quota i.
func logSpent(ctx context.Context, i int, b prefetchBudget, budget int64) {
	q := budgetQuotas[i]
	slog.InfoContext(ctx, "prefetch budget spent", "span", "prefetch", "api", q.api, "resource", q.resource,
		"used", b.used, "budget", budget, "until", b.renew)
}

// checkPrefetch marks the budget of quota i spent once its use reached
// budget, until the quota refills at reset, and not spent while it is
// under budget, which may change while the app runs, or nothing was
// spent. It reports whether the budget became spent now. s.budgetMu must
// be held.
func (s *Stats) checkPrefetch(i int, budget int64, reset, now time.Time) (over bool) {
	s.renewPrefetch(i, now)
	b := &s.budget[i]
	switch {
	case b.used < budget || b.used == 0:
		b.spent = false
	case !b.spent:
		b.spent, b.renew, over = true, windowEnd(reset, now), true
	}
	return over
}

// windowEnd returns when the window of a quota that GitHub said refills
// at reset ends: at reset, or fallbackPrefetchWindow on if it said none.
func windowEnd(reset, now time.Time) time.Time {
	if !reset.After(now) {
		return now.Add(fallbackPrefetchWindow)
	}
	return reset
}

// renewPrefetch starts the budget of quota i over once the window of the
// quota its use counts in has ended, spent or not. s.budgetMu must be
// held.
func (s *Stats) renewPrefetch(i int, now time.Time) {
	if b := s.budget[i]; b.used > 0 && !now.Before(b.renew) {
		s.budget[i] = prefetchBudget{}
	}
}

// prefetchLimit returns what the reads ahead may spend of quota i, and
// when the quota GitHub last reported refills, or the zero time.
func (s *Stats) prefetchLimit(i int) (budget int64, reset time.Time) {
	var last Rate
	s.mu.Lock()
	if q := s.quotas[budgetQuotas[i]]; q != nil {
		last = q.last
	}
	s.mu.Unlock()
	limit := int64(fallbackLimit)
	if last.Limit > 0 {
		limit, reset = int64(last.Limit), last.Reset
	}
	return limit * prefetchShare.Load() / 100, reset
}
