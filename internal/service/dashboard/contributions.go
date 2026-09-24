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

// Contributions returns the viewer's contribution calendar for the past
// year. It is fresh for ContributionsTTL, and is served stale or offline
// as in Header.
func (s *Service) Contributions(ctx context.Context) (core.Contributions, error) {
	c, err := s.contributions.get(ctx, contributionsKey, s.api.ViewerContributions)
	if err != nil {
		return core.Contributions{}, fmt.Errorf("dashboard contributions: %w", err)
	}
	return c, nil
}
