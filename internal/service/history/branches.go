package history

import (
	"context"
	"fmt"
	"strconv"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/fallback"
)

// DefaultBranchPageSize is the page size of a BranchesQuery that sets
// none: branches are small, so a page holds as many as GitHub lists.
const DefaultBranchPageSize = 100

// BranchesQuery selects a page of a repository's branches.
type BranchesQuery struct {
	Repo core.RepoRef
	// Cursor is the Next of the previous page, or empty for the first.
	Cursor string
	// PageSize defaults to DefaultBranchPageSize and is at most 100.
	PageSize int
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

func (q BranchesQuery) normalize() BranchesQuery {
	q.PageSize = pageSize(q.PageSize, DefaultBranchPageSize)
	return q
}

// CachedBranches returns the cached page for q, fresh or stale, without a
// request. It reports false if the page isn't in memory.
func (s *Service) CachedBranches(q BranchesQuery) (core.Page[core.Branch], bool) {
	e, st := s.branches.Get(branchesKey(q.normalize()))
	return e.Value, st != cache.Miss
}

// Branches returns a page of branches, by name. A fresh page comes from
// the cache; otherwise it is fetched, conditionally if a stale copy is
// cached.
//
// A page that only an earlier session kept is fresh if it was fetched or
// revalidated within the TTL. An older one is returned at once, with Stale
// set, to every read until one with q.Again set revalidates it. If GitHub
// can't be reached, a stale page is served with Offline set, and if it
// rate limits the read, with Limited set.
func (s *Service) Branches(ctx context.Context, q BranchesQuery) (core.Page[core.Branch], error) {
	q = q.normalize()
	key := branchesKey(q)
	if e, ok := s.keptBranches.Warm(s.branches, key, q.Again); ok {
		p := e.Value
		p.Stale = true
		return p, nil
	}
	p, err := fetch(ctx, s.branches, s.keptBranches, key, []string{repoTag(q.Repo)}, fallback.Page[core.Branch], s.loadBranches(q))
	if err != nil {
		return core.Page[core.Branch]{}, fmt.Errorf("list branches of %s: %w", q.Repo, err)
	}
	return p, nil
}

func (s *Service) loadBranches(q BranchesQuery) func(ctx context.Context, cond github.Conditional) (core.Page[core.Branch], github.Response, error) {
	return func(ctx context.Context, cond github.Conditional) (core.Page[core.Branch], github.Response, error) {
		return s.api.ListBranches(ctx, q.Repo, q.Cursor, q.PageSize, cond)
	}
}

// branchesKey keys a page of branches. The cursor is a URL, and may hold
// colons, so it comes last.
func branchesKey(q BranchesQuery) string {
	return "branches:" + repoKey(q.Repo) + ":" + strconv.Itoa(q.PageSize) + ":" + q.Cursor
}
