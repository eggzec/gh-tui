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
	// Filter narrows the list further, in GitHub's search syntax without
	// the repository and the state, such as "label:bug assignee:@me
	// sort:created-asc". Empty lists them all, most recently updated
	// first. See List for how it is read.
	Filter string
	// Cursor is the Next of the previous page, or empty for the first.
	Cursor string
	// PageSize defaults to DefaultPageSize and is at most 100. A
	// cursor keeps the size of the page it came from, so it sizes only the
	// first page, but every size is cached apart.
	PageSize int
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
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

// FreshList reports whether the page for q is cached and fresh, so that
// List returns it without a request: read this session, or kept by an
// earlier one and fetched or revalidated within the TTL. A page kept longer
// ago is put in memory stale, so that a List with q.Again set revalidates
// it with its validators. It may read the store, so call it where I/O is
// fine, such as in a tea.Cmd.
func (s *Service) FreshList(q ListQuery) bool {
	key := listKey(q.normalize())
	s.keptLists.Warm(s.lists, key, true)
	return fresh(s.lists, key)
}

// List returns a page of issues, most recently updated first unless the
// filter sorts otherwise. A fresh page comes from the cache; otherwise it
// is fetched, conditionally if a stale copy is cached. A page may be short,
// or empty, and still have a Next.
//
// The repository's list answers a filter of label:, assignee:, author:,
// mentions:, no:assignee, no:milestone and sort: qualifiers, with @me for
// the signed-in user. Any other filter is a search, which GitHub limits to
// 30 a minute and can't answer with a free 304. Only pages without a
// filter are kept for later sessions.
//
// A page that only an earlier session kept is fresh if it was fetched or
// revalidated within the TTL. An older one is returned at once, with Stale
// set, to every read until one with q.Again set revalidates it. If GitHub
// can't be reached, a stale page is served with Offline set.
func (s *Service) List(ctx context.Context, q ListQuery) (core.Page[core.Issue], error) {
	q = q.normalize()
	key := listKey(q)
	shelf := s.keptLists
	if q.Filter != "" {
		// Filters are many and short-lived, so only the lists every
		// visit starts from are kept.
		shelf = nil
	}
	if e, ok := shelf.Warm(s.lists, key, q.Again); ok {
		// The page vouches for what is cached of its issues as of when it
		// was read, like the other pages shown with it.
		s.vouch(q.Repo, e.Value.Items)
		p := e.Value
		p.Stale = true
		return p, nil
	}
	page, err := fetch(ctx, s.lists, shelf, key, listTags(q.Repo), offlinePage[core.Issue],
		func(ctx context.Context, cond github.Conditional) (core.Page[core.Issue], github.Response, error) {
			return s.readList(ctx, q, cond)
		})
	if err != nil {
		if github.Refused(err) {
			// A kept page may have vouched for what is cached of the
			// repository's issues.
			s.seen.DeletePrefix(issuePrefix(q.Repo))
		}
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
// stale copy is cached. What an earlier session kept counts as cached, and
// is served if GitHub can't be reached.
func (s *Service) Get(ctx context.Context, repo core.RepoRef, number int) (core.Issue, error) {
	key := issueKey(repo, number)
	s.keptIssues.Warm(s.issues, key, true)
	if it, ok := s.currentIssue(key); ok {
		s.issues.Hit(key)
		return it, nil
	}
	it, err := fetch(ctx, s.issues, s.keptIssues, key,
		func(core.Issue) []string { return []string{repoTag(repo), key} }, asIs[core.Issue],
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
// What an earlier session kept counts as cached, as for Get.
func (s *Service) Comments(ctx context.Context, q CommentsQuery) (core.Page[core.Comment], error) {
	q = q.normalize()
	ckey := commentsKey(q)
	s.keptComments.Warm(s.comments, ckey, true)
	if p, ok := s.currentComments(q); ok {
		s.comments.Hit(ckey)
		return p, nil
	}
	key := issueKey(q.Repo, q.Number)
	// The page is at least as recent as what the list showed before the
	// read.
	version, _ := s.seen.Get(key)
	tags := []string{repoTag(q.Repo), key}
	e, err := s.comments.Fetch(ctx, ckey, func(ctx context.Context, prev cache.Entry[stampedComments], ok bool) (cache.Entry[stampedComments], error) {
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		p, res, err := s.api.ListIssueComments(ctx, q.Repo, q.Number, q.Cursor, q.PageSize, cond)
		switch {
		case ok && github.Unreachable(ctx, err):
			prev.Value.Value.Offline, prev.FetchedAt = true, offlineAt
			return prev, nil
		case err != nil:
			if github.Refused(err) {
				s.keptComments.Delete(ckey)
			}
			return prev, err
		case res.NotModified:
			// The cached page is current as of now, so it takes the newer
			// version. It may hold a comment GitHub hasn't confirmed, so
			// it isn't kept again; the kept page stays as GitHub sent it.
			prev.Value.Version, prev.FetchedAt = version, time.Time{}
			return prev, nil
		}
		e := cache.Entry[stampedComments]{
			Value: stampedComments{Value: p, Version: version},
			ETag:  res.ETag, LastModified: res.LastModified, Source: res.URL, Tags: tags,
		}
		_ = s.keptComments.Save(ckey, e)
		return e, nil
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
