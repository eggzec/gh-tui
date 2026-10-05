package owners

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
)

func contributionsKey(login string) string {
	return "ownercontrib:" + loginKey(login)
}

// CachedContributions returns the cached contribution calendar of the
// user login, fresh or stale, without I/O.
func (s *Service) CachedContributions(login string) (core.Contributions, bool) {
	return s.contributions.cached(contributionsKey(login))
}

// FreshContributions reports whether the contribution calendar of the user
// login is cached and fresh, so that reading it costs no request. It does
// no I/O.
func (s *Service) FreshContributions(login string) bool {
	return s.contributions.fresh(contributionsKey(login))
}

// ContributionsQuery selects the contribution calendar of a user.
type ContributionsQuery struct {
	Login string
	// Again reads past a kept calendar: set it on the read that follows
	// one that came back Stale. It doesn't key the cache.
	Again bool
}

// Contributions returns the contribution calendar of the user q.Login for
// the past year. It is fresh for TTLs.Contributions, and is served stale,
// offline or limited, or fails for a user that isn't there, as in Header.
func (s *Service) Contributions(ctx context.Context, q ContributionsQuery) (core.Contributions, error) {
	// The client already names the request in its error.
	return read(ctx, s, &s.contributions, q.Login, "contributions", contributionsKey(q.Login), q.Again, func(ctx context.Context) (core.Contributions, error) {
		return s.api.UserContributions(ctx, q.Login)
	})
}
