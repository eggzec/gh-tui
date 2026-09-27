package obs

import (
	"context"
	"log/slog"
	"time"
)

// Reads ahead, such as the details of the first rows of a list, are
// guesses, so they may spend only a share of the hourly GraphQL points:
// prefetchShare percent of the limit GitHub last reported, or
// fallbackPrefetchPoints before it reported one. Once they spent it, no
// more are read ahead until the GraphQL quota GitHub reported refills, or
// for fallbackPrefetchWindow if it reported none; then they start over.
// What the user asks for is never held back. REST reads cost no points,
// and GraphQL queries whose cost is unknown count as one point, the least
// a query costs.
const (
	prefetchShare          = 10
	fallbackPrefetchPoints = 500
	fallbackPrefetchWindow = time.Hour
)

// prefetchBudget is what the reads ahead spent of their budget.
type prefetchBudget struct {
	points int64
	// spent is set once points reached the budget, until renew.
	spent bool
	renew time.Time
}

// ChargeGraphQL counts a GraphQL query sent with ctx that cost cost points,
// or 0 if unknown, against the default stats' prefetch budget, if ctx is a
// read ahead (ForPrefetch).
func ChargeGraphQL(ctx context.Context, cost int) {
	if IsPrefetch(ctx) {
		Default().chargePrefetch(ctx, cost)
	}
}

// PrefetchSpent reports whether the reads ahead spent their budget, in the
// default stats, until the GraphQL quota refills.
func PrefetchSpent() bool {
	s := Default()
	s.budgetMu.Lock()
	defer s.budgetMu.Unlock()
	s.renewPrefetch(time.Now())
	return s.budget.spent
}

func (s *Stats) chargePrefetch(ctx context.Context, cost int) {
	budget, reset := s.prefetchLimit()
	now := time.Now()
	s.budgetMu.Lock()
	s.renewPrefetch(now)
	s.budget.points += int64(max(cost, 1))
	points, over := s.budget.points, !s.budget.spent && s.budget.points >= budget
	if over {
		if !reset.After(now) {
			reset = now.Add(fallbackPrefetchWindow)
		}
		s.budget.spent, s.budget.renew = true, reset
	}
	s.budgetMu.Unlock()
	if over {
		slog.InfoContext(ctx, "prefetch budget spent", "span", "prefetch", "points", points, "budget", budget,
			"until", reset)
	}
}

// renewPrefetch starts the budget over once the quota it was spent in has
// refilled. s.budgetMu must be held.
func (s *Stats) renewPrefetch(now time.Time) {
	if s.budget.spent && !now.Before(s.budget.renew) {
		s.budget = prefetchBudget{}
	}
}

// prefetchLimit returns the GraphQL points the reads ahead may spend, and
// when the quota GitHub last reported refills, or the zero time.
func (s *Stats) prefetchLimit() (budget int64, reset time.Time) {
	var last Rate
	s.mu.Lock()
	if q := s.quotas[quotaKey{GraphQL, "graphql"}]; q != nil {
		last = q.last
	}
	s.mu.Unlock()
	if last.Limit <= 0 {
		return fallbackPrefetchPoints, time.Time{}
	}
	return int64(last.Limit) * prefetchShare / 100, last.Reset
}
