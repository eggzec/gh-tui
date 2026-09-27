package github

import (
	"cmp"
	"slices"

	"github.com/eggzec/gh-tui/internal/core"
)

// resourceOrder is the order the resources gh-tui uses most are shown in.
// Others follow by name.
var resourceOrder = []string{resourceCore, resourceGraphQL, resourceSearch, resourceCodeSearch}

// RateStatus returns the rate limits of the account as the client knows
// them now, with no request.
func (c *Client) RateStatus() core.RateStatus {
	return c.budget.snapshot()
}

// snapshot returns every quota at one moment, taken under one lock so
// that none is from before an answer that another reflects.
func (b *budget) snapshot() core.RateStatus {
	now := b.now()
	b.mu.Lock()
	defer b.mu.Unlock()
	s := core.RateStatus{
		Quotas:   make([]core.Quota, 0, len(b.quotas)),
		Answered: b.answered,
		Failed:   b.failed,
		At:       now,
		// SecondaryUntil stays zero while no secondary limit holds
		// requests back.
	}
	for resource, q := range b.quotas {
		cq := core.Quota{
			Resource:  resource,
			Limit:     q.limit,
			Remaining: max(b.est(resource, q), 0),
			Reset:     b.local(q.reset),
			SeenAt:    q.seenAt,
			// Held stays zero while the budget only counts requests
			// and never holds one back for a release.
		}
		if q.release.After(now) {
			cq.LimitedUntil = q.release
		}
		s.Quotas = append(s.Quotas, cq)
	}
	slices.SortFunc(s.Quotas, func(x, y core.Quota) int {
		return cmp.Or(cmp.Compare(resourceRank(x.Resource), resourceRank(y.Resource)), cmp.Compare(x.Resource, y.Resource))
	})
	return s
}

// resourceRank is where resource comes in resourceOrder, or after them
// all.
func resourceRank(resource string) int {
	if i := slices.Index(resourceOrder, resource); i >= 0 {
		return i
	}
	return len(resourceOrder)
}
