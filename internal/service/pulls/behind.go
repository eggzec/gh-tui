package pulls

import (
	"context"
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// BehindQuery selects the commits of a base that a head lacks, which are what
// "Update branch" would bring into a pull request that is behind.
type BehindQuery struct {
	Repo core.RepoRef
	// Head and Base are full commit SHAs: the pull request's head, and the tip
	// of its base. What lies between two commits never changes, so a push to
	// either is a new key.
	Head, Base string
}

// key is the key of the commits of q.
func (q BehindQuery) key() string {
	return "pullbehind:" + repoID(q.Repo) + "@" + strings.ToLower(q.Head) + "..." + strings.ToLower(q.Base)
}

// CachedBehind returns the commits of q if they are in memory, without
// fetching them.
func (s *Service) CachedBehind(q BehindQuery) (core.BaseCommits, bool) {
	return cached(s.behinds, q.key())
}

// Behind returns the commits that q.Base has and q.Head lacks: the first page
// of them, with how many there are in all. They are cached for good, in memory
// and in the store, since the SHAs name them. It fails if either isn't a full
// SHA.
func (s *Service) Behind(ctx context.Context, q BehindQuery) (core.BaseCommits, error) {
	for _, sha := range []string{q.Head, q.Base} {
		if !isSHA(sha) {
			return core.BaseCommits{}, fmt.Errorf("list commits of %s behind %s of %s: %w", q.Base, q.Head, q.Repo, errNotSHA)
		}
	}
	key := q.key()
	e, err := s.behinds.Fetch(ctx, key, func(ctx context.Context, _ cache.Entry[core.BaseCommits], _ bool) (cache.Entry[core.BaseCommits], error) {
		if kept, ok := s.keptBehind.Load(key); ok {
			return cache.Entry[core.BaseCommits]{Value: kept.Value}, nil
		}
		b, err := s.api.BehindCommits(ctx, q.Repo, q.Head, q.Base)
		if err != nil {
			return cache.Entry[core.BaseCommits]{}, err
		}
		e := cache.Entry[core.BaseCommits]{Value: b}
		// The store is only a shortcut, so a failure is ignored.
		_ = s.keptBehind.Save(key, e)
		return e, nil
	})
	if err != nil {
		return core.BaseCommits{}, fmt.Errorf("list commits of %s behind %s of %s: %w", q.Base, q.Head, q.Repo, err)
	}
	return e.Value, nil
}
