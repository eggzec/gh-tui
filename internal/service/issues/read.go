package issues

import (
	"cmp"
	"context"
	"fmt"
	"time"

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
	s.vouch(q.Repo, page.Items)
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
// A fresh or current issue comes from the cache, current meaning as recent
// as the list last showed it; otherwise it is fetched, conditionally if a
// stale copy is cached.
func (s *Service) Get(ctx context.Context, repo core.RepoRef, number int) (core.Issue, error) {
	key := issueKey(repo, number)
	if it, ok := s.currentIssue(key); ok {
		return it, nil
	}
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
	return e.Value.Value, st != cache.Miss
}

// Comments returns a page of the comments on an issue, oldest first. Each
// page is cached and revalidated on its own, like List's, and is current
// while it was read at the version of the issue that the list last showed.
func (s *Service) Comments(ctx context.Context, q CommentsQuery) (core.Page[core.Comment], error) {
	q = q.normalize()
	if p, ok := s.currentComments(q); ok {
		return p, nil
	}
	key := issueKey(q.Repo, q.Number)
	// The page is at least as recent as what the list showed before the
	// read.
	version, _ := s.seen.Get(key)
	tags := []string{repoTag(q.Repo), key}
	e, err := s.comments.Fetch(ctx, commentsKey(q), func(ctx context.Context, prev cache.Entry[stampedComments], ok bool) (cache.Entry[stampedComments], error) {
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		p, res, err := s.api.ListIssueComments(ctx, q.Repo, q.Number, q.Cursor, q.PageSize, cond)
		switch {
		case err != nil:
			return prev, err
		case res.NotModified:
			// The cached page is current as of now, so it takes the newer
			// version.
			prev.Value.Version, prev.FetchedAt = version, time.Time{}
			return prev, nil
		}
		return cache.Entry[stampedComments]{
			Value: stampedComments{Value: p, Version: version},
			ETag:  res.ETag, LastModified: res.LastModified, Tags: tags,
		}, nil
	})
	if err != nil {
		return core.Page[core.Comment]{}, fmt.Errorf("list comments of issue %s#%d: %w", q.Repo, q.Number, err)
	}
	return e.Value.Value, nil
}

// Invalidate marks everything cached of repo stale: its list pages, issues
// and comments. They are still served by the Cached reads, and the next
// fetch of each asks GitHub, conditionally, so a refresh reaches the server
// even while the entries are fresh, and costs no rate limit if nothing
// changed.
func (s *Service) Invalidate(repo core.RepoRef) {
	s.seen.DeletePrefix(issuePrefix(repo))
	tag := repoTag(repo)
	s.lists.InvalidateTag(tag)
	s.issues.InvalidateTag(tag)
	s.comments.InvalidateTag(tag)
}
