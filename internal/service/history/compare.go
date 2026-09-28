package history

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/fallback"
)

// CachedCompare returns the cached comparison of head with base, fresh or
// stale, without a request. It reports false if it isn't in memory.
func (s *Service) CachedCompare(repo core.RepoRef, base, head string) (core.Compare, bool) {
	e, st := s.compares.Get(compareKey(repo, base, head))
	return e.Value, st != cache.Miss
}

// Compare returns how far head is from base, each a branch, a tag or a
// commit SHA. It is cached in memory for a short TTL, and revalidated with
// its ETag after it. If GitHub can't be reached, the stale comparison is
// served.
func (s *Service) Compare(ctx context.Context, repo core.RepoRef, base, head string) (core.Compare, error) {
	c, err := fetch(ctx, s.compares, nil, compareKey(repo, base, head), []string{repoTag(repo)}, fallback.None[core.Compare],
		func(ctx context.Context, cond github.Conditional) (core.Compare, github.Response, error) {
			return s.api.Compare(ctx, repo, base, head, cond)
		})
	if err != nil {
		return core.Compare{}, fmt.Errorf("compare %s...%s of %s: %w", base, head, repo, err)
	}
	return c, nil
}

func compareKey(repo core.RepoRef, base, head string) string {
	return "compare:" + repoKey(repo) + ":" + base + "..." + head
}
