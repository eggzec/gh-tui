package issues

import (
	"cmp"
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// ListQuery selects a page of a repository's issues.
type ListQuery struct {
	Repo core.RepoRef
	// State defaults to core.FilterOpen.
	State core.StateFilter
	// Cursor is the Next of the previous page, or empty for the first.
	Cursor string
	// PageSize defaults to DefaultPageSize and is at most 100. A
	// cursor keeps the size of the page it came from, so it sizes only the
	// first page, but every size is cached apart.
	PageSize int
}

func (q ListQuery) normalize() ListQuery {
	q.State = cmp.Or(q.State, core.FilterOpen)
	q.PageSize = pageSize(q.PageSize)
	return q
}

// Page sizes of the queries.
const (
	DefaultPageSize = 30
	// maxPageSize is the most GitHub returns in one page.
	maxPageSize = 100
)

func pageSize(n int) int {
	if n <= 0 {
		return DefaultPageSize
	}
	return min(n, maxPageSize)
}

// CachedList returns the cached page for q, fresh or stale, without a
// request. It reports false if the page isn't cached.
func (s *Service) CachedList(q ListQuery) (core.Page[core.Issue], bool) {
	e, st := s.lists.Get(listKey(q.normalize()))
	return e.Value, st != cache.Miss
}

// List returns a page of issues, most recently updated first. A fresh page
// comes from the cache; otherwise it is fetched, conditionally if a stale
// copy is cached. A page may be short, or empty, and still have a Next.
func (s *Service) List(ctx context.Context, q ListQuery) (core.Page[core.Issue], error) {
	q = q.normalize()
	page, err := fetch(ctx, s.lists, listKey(q), listTags(q.Repo),
		func(ctx context.Context, cond github.Conditional) (core.Page[core.Issue], github.Response, error) {
			return s.api.ListIssues(ctx, q.Repo, q.State, q.Cursor, q.PageSize, cond)
		})
	if err != nil {
		return core.Page[core.Issue]{}, fmt.Errorf("list issues of %s: %w", q.Repo, err)
	}
	return page, nil
}

func listTags(repo core.RepoRef) func(core.Page[core.Issue]) []string {
	return func(p core.Page[core.Issue]) []string {
		tags := make([]string, 0, len(p.Items)+1)
		tags = append(tags, repoTag(repo))
		for i := range p.Items {
			tags = append(tags, issueKey(repo, p.Items[i].Number))
		}
		return tags
	}
}

// CachedGet returns the cached issue, fresh or stale, without a request. It
// reports false if the issue isn't cached.
func (s *Service) CachedGet(repo core.RepoRef, number int) (core.Issue, bool) {
	e, st := s.issues.Get(issueKey(repo, number))
	return e.Value, st != cache.Miss
}

// Get returns an issue without its comments, which Comments pages through.
// A fresh issue comes from the cache; otherwise it is fetched, conditionally
// if a stale copy is cached.
func (s *Service) Get(ctx context.Context, repo core.RepoRef, number int) (core.Issue, error) {
	key := issueKey(repo, number)
	it, err := fetch(ctx, s.issues, key,
		func(core.Issue) []string { return []string{repoTag(repo), key} },
		func(ctx context.Context, cond github.Conditional) (core.Issue, github.Response, error) {
			return s.api.GetIssue(ctx, repo, number, cond)
		})
	if err != nil {
		return core.Issue{}, fmt.Errorf("get issue %s#%d: %w", repo, number, err)
	}
	return it, nil
}

// CommentsQuery selects a page of the comments on an issue.
type CommentsQuery struct {
	Repo   core.RepoRef
	Number int
	// Cursor and PageSize work as in ListQuery.
	Cursor   string
	PageSize int
}

func (q CommentsQuery) normalize() CommentsQuery {
	q.PageSize = pageSize(q.PageSize)
	return q
}

// CachedComments returns the cached page for q, fresh or stale, without a
// request. It reports false if the page isn't cached.
func (s *Service) CachedComments(q CommentsQuery) (core.Page[core.Comment], bool) {
	e, st := s.comments.Get(commentsKey(q.normalize()))
	return e.Value, st != cache.Miss
}

// Comments returns a page of the comments on an issue, oldest first. Each
// page is cached and revalidated on its own, like List's.
func (s *Service) Comments(ctx context.Context, q CommentsQuery) (core.Page[core.Comment], error) {
	q = q.normalize()
	tags := []string{repoTag(q.Repo), issueKey(q.Repo, q.Number)}
	page, err := fetch(ctx, s.comments, commentsKey(q),
		func(core.Page[core.Comment]) []string { return tags },
		func(ctx context.Context, cond github.Conditional) (core.Page[core.Comment], github.Response, error) {
			return s.api.ListIssueComments(ctx, q.Repo, q.Number, q.Cursor, q.PageSize, cond)
		})
	if err != nil {
		return core.Page[core.Comment]{}, fmt.Errorf("list comments of issue %s#%d: %w", q.Repo, q.Number, err)
	}
	return page, nil
}
