package history

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// DefaultCommitPageSize is the page size of a CommitsQuery that sets none.
const DefaultCommitPageSize = 50

// CommitsQuery selects a page of the history of a ref.
type CommitsQuery struct {
	Repo core.RepoRef
	// Ref is a branch, a tag or a commit SHA. Empty means the default
	// branch.
	Ref string
	// Cursor is the Next of the previous page, or empty for the first. A
	// cursor names the commit the first page started at, so it reads the
	// same commits for good, whatever Ref points at meanwhile.
	Cursor string
	// PageSize defaults to DefaultCommitPageSize and is at most 100. A
	// cursor keeps the size of the page it came from.
	PageSize int
}

func (q CommitsQuery) normalize() CommitsQuery {
	q.PageSize = pageSize(q.PageSize, DefaultCommitPageSize)
	if isSHA(q.Ref) {
		q.Ref = strings.ToLower(q.Ref)
	}
	return q
}

// pinned reports whether a SHA names the page of q, so it never changes.
func (q CommitsQuery) pinned() bool {
	return q.Cursor != "" || isSHA(q.Ref)
}

// CachedCommits returns the cached page for q, fresh or stale, without a
// request. It reports false if the page isn't in memory.
func (s *Service) CachedCommits(q CommitsQuery) (core.Page[core.Commit], bool) {
	q = q.normalize()
	c, key := s.refPages, refPageKey(q)
	if q.pinned() {
		c, key = s.pages, pageKey(q)
	}
	e, st := c.Get(key)
	return e.Value, st != cache.Miss
}

// Commits returns a page of the history of q.Ref, newest first, as git log
// walks it, with each commit's parents to draw the graph. An empty
// repository has an empty history.
//
// The first page of a branch or tag is revalidated like Branches, and
// served Stale or Offline the same way. Pages that a SHA names, the first
// page of a commit and every page after the first, are cached for good.
func (s *Service) Commits(ctx context.Context, q CommitsQuery) (core.Page[core.Commit], error) {
	q = q.normalize()
	var (
		p   core.Page[core.Commit]
		err error
	)
	if q.pinned() {
		p, err = object(ctx, s.pages, s.keptPages, pageKey(q), func(ctx context.Context) (core.Page[core.Commit], error) {
			p, _, err := s.api.ListCommits(ctx, q.Repo, q.Ref, q.Cursor, q.PageSize, github.Conditional{})
			return p, err
		})
	} else {
		key := refPageKey(q)
		if e, ok := s.keptRefPages.Warm(s.refPages, key); ok {
			p := e.Value
			p.Stale = true
			return p, nil
		}
		p, err = fetch(ctx, s.refPages, s.keptRefPages, key, []string{repoTag(q.Repo)}, offlinePage[core.Commit], s.loadRefPage(q))
	}
	if err != nil {
		return core.Page[core.Commit]{}, fmt.Errorf("list commits of %s at %s: %w", q.Repo, refName(q.Ref), err)
	}
	return p, nil
}

func (s *Service) loadRefPage(q CommitsQuery) func(ctx context.Context, cond github.Conditional) (core.Page[core.Commit], github.Response, error) {
	return func(ctx context.Context, cond github.Conditional) (core.Page[core.Commit], github.Response, error) {
		return s.api.ListCommits(ctx, q.Repo, q.Ref, "", q.PageSize, cond)
	}
}

func refName(ref string) string {
	if ref == "" {
		return "the default branch"
	}
	return ref
}

// refPageKey keys the first page of the history of a ref. Git refs can't
// hold colons, but the ref comes last anyway.
func refPageKey(q CommitsQuery) string {
	return "commits:" + q.Repo.String() + ":" + strconv.Itoa(q.PageSize) + ":" + q.Ref
}

// pageKey keys a page that a SHA names: by its cursor, which names the SHA
// and the page size, or by the SHA and the size for a first page.
func pageKey(q CommitsQuery) string {
	if q.Cursor != "" {
		return "commitpage:" + repoKey(q.Repo) + ":" + q.Cursor
	}
	return "commitpage:" + repoKey(q.Repo) + ":" + strconv.Itoa(q.PageSize) + ":" + q.Ref
}
