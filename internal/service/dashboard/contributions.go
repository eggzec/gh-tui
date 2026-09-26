package dashboard

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
)

const contributionsKey = "dashcontrib:viewer"

// CachedContributions returns the cached contribution calendar, fresh or
// stale, without I/O.
func (s *Service) CachedContributions() (core.Contributions, bool) {
	return s.contributions.cached(contributionsKey)
}

// ContributionsQuery selects how the contribution calendar is read.
type ContributionsQuery struct {
	// Again reads past a kept calendar: set it on the read that follows
	// one that came back Stale.
	Again bool
}

// Contributions returns the viewer's contribution calendar for the past
// year. It is fresh for ContributionsTTL, and is served stale or offline
// as in Header.
func (s *Service) Contributions(ctx context.Context, q ContributionsQuery) (core.Contributions, error) {
	c, err := s.contributions.get(ctx, contributionsKey, q.Again, s.api.ViewerContributions)
	if err != nil {
		return core.Contributions{}, fmt.Errorf("dashboard contributions: %w", err)
	}
	return c, nil
}
