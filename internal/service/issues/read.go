package issues

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"sync"

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

// CachedGet returns the cached detail of an issue, fresh or stale,
// without a request. It reports false unless both the issue and its
// comments are cached.
func (s *Service) CachedGet(repo core.RepoRef, number int) (core.IssueDetail, bool) {
	it, st := s.issues.Get(issueKey(repo, number))
	if st == cache.Miss {
		return core.IssueDetail{}, false
	}
	thread, st := s.comments.Get(commentsKey(repo, number))
	if st == cache.Miss {
		return core.IssueDetail{}, false
	}
	return core.IssueDetail{Issue: it.Value, Thread: thread.Value.Items}, true
}

// Get returns an issue with its comments. The issue and its comments are
// cached and revalidated separately, and fetched in parallel.
func (s *Service) Get(ctx context.Context, repo core.RepoRef, number int) (core.IssueDetail, error) {
	key := issueKey(repo, number)
	tags := func(core.Issue) []string { return []string{repoTag(repo), key} }
	var (
		wg    sync.WaitGroup
		it    core.Issue
		itErr error
	)
	wg.Go(func() {
		it, itErr = fetch(ctx, s.issues, key, tags,
			func(ctx context.Context, cond github.Conditional) (core.Issue, github.Response, error) {
				return s.api.GetIssue(ctx, repo, number, cond)
			})
	})
	thread, threadErr := fetch(ctx, s.comments, commentsKey(repo, number),
		func(core.Page[core.Comment]) []string { return []string{repoTag(repo), key} },
		func(ctx context.Context, cond github.Conditional) (core.Page[core.Comment], github.Response, error) {
			return s.api.ListIssueComments(ctx, repo, number, "", cond)
		})
	wg.Wait()
	if err := errors.Join(itErr, threadErr); err != nil {
		return core.IssueDetail{}, fmt.Errorf("get issue %s#%d: %w", repo, number, err)
	}
	return core.IssueDetail{Issue: it, Thread: thread.Items}, nil
}
