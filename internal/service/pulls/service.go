// Package pulls serves pull requests from a cache in front of the GitHub
// API, and applies changes to them optimistically.
package pulls

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/probe"
	"github.com/eggzec/gh-tui/internal/service/seen"
)

// API is the part of the GitHub client the service uses. The mutations take
// the node ID of the pull request and return it as the server left it.
// ProbePullRequests is a cheap conditional request that Poll watches. The
// comments of a pull request are those of its issue, which REST reads.
type API interface {
	ListPullRequests(ctx context.Context, repo core.RepoRef, state core.State, cursor string, first int) (core.Page[core.PullRequest], error)
	FilterPullRequests(ctx context.Context, repo core.RepoRef, f github.PullFilter, cursor string, first int) (core.Page[core.PullRequest], error)
	SearchPullRequests(ctx context.Context, query, cursor string, first int) (core.Page[core.PullRequest], error)
	GetPullRequest(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error)
	ListIssueComments(ctx context.Context, repo core.RepoRef, number int, cursor string, perPage int, cond github.Conditional) (core.Page[core.Comment], github.Response, error)
	ListPullRequestReviews(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.Review], error)
	PullRequestID(ctx context.Context, repo core.RepoRef, number int) (string, error)
	MergePullRequest(ctx context.Context, id string, method core.MergeMethod) (core.PullRequest, error)
	ClosePullRequest(ctx context.Context, id string) (core.PullRequest, error)
	ReopenPullRequest(ctx context.Context, id string) (core.PullRequest, error)
	MarkPullRequestReady(ctx context.Context, id string) (core.PullRequest, error)
	ConvertPullRequestToDraft(ctx context.Context, id string) (core.PullRequest, error)
	ProbePullRequests(ctx context.Context, repo core.RepoRef, cond github.Conditional) (github.Response, error)
}

// Service reads pull requests through a cache and changes them
// optimistically. It is safe for concurrent use.
type Service struct {
	api     API
	lists   *cache.Cache[listPage]
	details *cache.Cache[core.PullRequestDetail]
	// Comments and reviews are cached a page at a time, so that the pages
	// the tui no longer shows are evicted. Comment pages carry the version
	// of the pull request they were read at, and REST's validators.
	comments *cache.Cache[stampedComments]
	reviews  *cache.Cache[core.Page[core.Review]]
	// The kept ones are what an earlier session read, if the service has
	// a store. A read that misses memory starts from them.
	keptLists    *cache.Shelf[listPage]
	keptDetails  *cache.Shelf[core.PullRequestDetail]
	keptComments *cache.Shelf[stampedComments]
	now          func() time.Time
	// ttl is how long a fetched entry stays fresh.
	ttl time.Duration
	// etags holds the latest probe ETag of each polled repository.
	etags probe.Tracker
	// seen holds what the list pages last showed of each pull request, by
	// detail key.
	seen seen.Marks[mark]
	// refreshes holds when the user last asked for each repository to be
	// read again.
	refreshes refreshes
}

// stampedComments is a cached page of comments with the version of its pull
// request.
type stampedComments = seen.Stamped[core.Page[core.Comment]]

// New returns a service that reads from api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	s := &Service{
		api:          api,
		lists:        cache.New[listPage](o.cache...),
		details:      cache.New[core.PullRequestDetail](o.cache...),
		comments:     cache.New[stampedComments](o.cache...),
		reviews:      cache.New[core.Page[core.Review]](o.cache...),
		keptLists:    cache.NewShelf[listPage](o.store, kindList, listSchema),
		keptDetails:  cache.NewShelf[core.PullRequestDetail](o.store, kindDetail, detailSchema),
		keptComments: cache.NewShelf[stampedComments](o.store, kindComments, commentsSchema),
		now:          time.Now,
		ttl:          cmp.Or(o.ttl, cache.DefaultTTL),
	}
	s.etags.Keep(o.store)
	return s
}

// The kinds of entries the service keeps in its store, and the version of
// the values of each. Bump the schema of a kind when its value changes
// shape, or what it means, so that older entries read as misses.
const (
	kindList     = "pulllist"
	kindDetail   = "pull"
	kindComments = "pullcomments"

	// listSchema 3 keeps when the page was last read in full.
	listSchema = 3
	// detailSchema 3 counts the checks rather than listing them.
	detailSchema = 3
	// commentsSchema 3 reads the pages with REST, whose cursors are URLs.
	commentsSchema = 3
)

// offlineAt is when an entry served offline was fetched, as far as the
// cache can tell: long ago, so it is stale at once and the next read asks
// GitHub again.
var offlineAt = time.Unix(1, 0)

// Page sizes. GitHub returns at most maxPageSize items per page.
const (
	defaultPageSize = 30
	maxPageSize     = 100
)

// pageSize returns n as a page size GitHub accepts: defaultPageSize if n is
// not positive, and at most maxPageSize.
func pageSize(n int) int {
	if n <= 0 {
		return defaultPageSize
	}
	return min(n, maxPageSize)
}

// ListQuery selects a page of the pull requests of a repository.
type ListQuery struct {
	Repo core.RepoRef
	// State filters by state. The empty state lists every pull request.
	State core.State
	// Filter narrows the list further, in GitHub's search syntax without
	// the repository and the state, such as "author:@me label:bug
	// sort:created-asc". Empty lists them all, most recently updated
	// first. See List for how it is read.
	Filter string
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many pull requests the page holds at most. Zero means
	// 30, and GitHub's maximum of 100 caps it.
	PageSize int
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

func (q ListQuery) key() string {
	v := url.Values{"state": {string(q.State)}, "cursor": {q.Cursor}, "first": {strconv.Itoa(pageSize(q.PageSize))}}
	if q.Filter != "" {
		v.Set("filter", q.Filter)
	}
	return "pulls:" + repoID(q.Repo) + "?" + v.Encode()
}

// repoID names a repository in keys and tags. GitHub ignores case in owner
// and repository names, so keys do too.
func repoID(r core.RepoRef) string {
	return strings.ToLower(r.String())
}

// repoTag tags every entry of a repository, so that a change to one of its
// pull requests finds the list pages that show it, and so that each cache
// can drop what it holds of a repository.
func repoTag(r core.RepoRef) string {
	return "repo:" + repoID(r)
}

// detailKey is the key of the detail of a pull request. It also tags every
// entry of the pull request, so that a change finds its comments.
func detailKey(r core.RepoRef, number int) string {
	return pullPrefix(r) + strconv.Itoa(number)
}

// pullPrefix starts the detail key of every pull request of r.
func pullPrefix(r core.RepoRef) string {
	return "pull:" + repoID(r) + "#"
}

// tags are the tags of an entry of pull request number of repo.
func tags(repo core.RepoRef, number int) []string {
	return []string{repoTag(repo), detailKey(repo, number)}
}

// cached returns the value under key in c, fresh or stale, without
// fetching it.
func cached[V any](c *cache.Cache[V], key string) (V, bool) {
	e, state := c.Get(key)
	return e.Value, state != cache.Miss
}

// fetch returns the value under key in c. A fresh value is returned without
// a request; otherwise load reads it, and it is stored and kept on shelf.
// What GitHub refuses is dropped from shelf. If GitHub can't be reached, the
// stale value is served instead, marked by offline.
func fetch[V any](ctx context.Context, c *cache.Cache[V], shelf *cache.Shelf[V], key string, offline func(V) V, load cache.FetchFunc[V]) (V, error) {
	e, err := c.Fetch(ctx, key, func(ctx context.Context, prev cache.Entry[V], ok bool) (cache.Entry[V], error) {
		e, err := load(ctx, prev, ok)
		switch {
		case ok && github.Unreachable(ctx, err):
			prev.Value, prev.FetchedAt = offline(prev.Value), offlineAt
			return prev, nil
		case errors.Is(err, cache.ErrNotModified):
			restamp(shelf, key, prev)
			return e, err
		case err != nil:
			if github.Refused(err) {
				shelf.Delete(key)
			}
			return cache.Entry[V]{}, err
		}
		// The shelf is only a shortcut, so a failure is ignored.
		_ = shelf.Save(key, e)
		return e, nil
	})
	return e.Value, err
}

// whole returns a load for fetch that reads the value in full with get, as
// GraphQL reads are, and tags it with tags.
func whole[V any](tags []string, get func(context.Context) (V, error)) cache.FetchFunc[V] {
	return func(ctx context.Context, _ cache.Entry[V], _ bool) (cache.Entry[V], error) {
		v, err := get(ctx)
		if err != nil {
			return cache.Entry[V]{}, err
		}
		return cache.Entry[V]{Value: v, Tags: tags}, nil
	}
}

// restamp marks what shelf keeps under key as fetched now, since GitHub
// confirmed prev, if it keeps the same. Prev may hold changes that GitHub
// hasn't confirmed, so it isn't kept itself.
func restamp[V any](shelf *cache.Shelf[V], key string, prev cache.Entry[V]) {
	kept, ok := shelf.Load(key)
	if !ok || kept.ETag != prev.ETag || kept.ETag == "" {
		return
	}
	kept.FetchedAt = time.Now()
	// The shelf is only a shortcut, so a failure is ignored.
	_ = shelf.Save(key, kept)
}

// offlineList marks a list page served offline.
func offlineList(p listPage) listPage {
	p.Page.Offline = true
	return p
}

// offlinePage marks a page served offline.
func offlinePage[T any](p core.Page[T]) core.Page[T] {
	p.Offline = true
	return p
}

// asIs serves a value offline unmarked.
func asIs[V any](v V) V { return v }

// CachedList returns the page for q if it is cached, fresh or stale, without
// fetching it.
func (s *Service) CachedList(q ListQuery) (core.Page[core.PullRequest], bool) {
	p, ok := cached(s.lists, q.key())
	return p.Page, ok
}

// FreshList reports whether the page for q is cached and fresh, so that
// List returns it without a request: read this session, or kept by an
// earlier one and fetched within the TTL. A page kept longer ago is put in
// memory stale, so that a List with q.Again set fetches it with its
// validators. It may read the store, so call it where I/O is fine, such as
// in a tea.Cmd.
func (s *Service) FreshList(q ListQuery) bool {
	key := q.key()
	s.keptLists.Warm(s.lists, key, true)
	return fresh(s.lists, key)
}

// List returns the page for q, most recently updated first unless its
// filter sorts otherwise. A fresh cached page is returned without a
// request. The page vouches for what is cached of the pull requests it
// lists: see [Service.Get].
//
// The repository's list answers a filter of one label: qualifier, base:,
// head: and sort:. Any other filter is a search, which costs the same one
// point. Only pages without a filter are kept for later sessions.
//
// A page that only an earlier session kept is fresh if it was fetched or
// revalidated within the TTL. An older one is returned at once, with Stale
// set, to every read until one with q.Again set fetches it, unless a free
// probe finds that no pull request of the repository changed since it was
// read: see loadList. If GitHub can't be reached, a stale page is served
// with Offline set.
func (s *Service) List(ctx context.Context, q ListQuery) (core.Page[core.PullRequest], error) {
	key := q.key()
	shelf := s.keptLists
	if q.Filter != "" {
		// Filters are many and short-lived, so only the lists every
		// visit starts from are kept.
		shelf = nil
	}
	if e, ok := shelf.Warm(s.lists, key, q.Again); ok {
		// The page vouches for what is cached of its pull requests as of
		// when it was read, like the other pages shown with it.
		s.vouch(q.Repo, e.Value.Page.Items)
		p := e.Value.Page
		p.Stale = true
		return p, nil
	}
	lp, err := fetch(ctx, s.lists, shelf, key, offlineList, s.loadList(q))
	p := lp.Page
	if err != nil {
		if github.Refused(err) {
			// A kept page may have vouched for what is cached of the
			// repository's pull requests.
			s.seen.DeletePrefix(pullPrefix(q.Repo))
		}
		return core.Page[core.PullRequest]{}, fmt.Errorf("list pulls of %s: %w", q.Repo, err)
	}
	s.vouch(q.Repo, p.Items)
	return p, nil
}

// CachedGet returns pull request number of repo if its detail is cached,
// fresh or stale, without fetching it.
func (s *Service) CachedGet(repo core.RepoRef, number int) (core.PullRequestDetail, bool) {
	return cached(s.details, detailKey(repo, number))
}

// Get returns pull request number of repo with its body and checks. Its
// comments and reviews are read with Comments and Reviews. A cached detail
// is returned without a request while it is fresh, or while it is current:
// as recent as the pull request the list last showed, with the same
// settled checks. What an earlier session kept counts as cached, and is
// served if GitHub can't be reached.
func (s *Service) Get(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error) {
	key := detailKey(repo, number)
	s.keptDetails.Warm(s.details, key, true)
	if d, ok := s.currentDetail(key); ok {
		s.details.Hit(key)
		return d, nil
	}
	d, err := fetch(ctx, s.details, s.keptDetails, key, asIs[core.PullRequestDetail], whole(tags(repo, number), func(ctx context.Context) (core.PullRequestDetail, error) {
		return s.api.GetPullRequest(ctx, repo, number)
	}))
	if err != nil {
		return core.PullRequestDetail{}, fmt.Errorf("get pull %s#%d: %w", repo, number, err)
	}
	return d, nil
}

// Invalidate marks everything cached of repo stale, for a refresh the user
// asked for: its list pages, details, comments and reviews. They are still
// served by the Cached reads, and the next fetch of each goes to GitHub, so
// a refresh reaches the server even while the entries are fresh or current.
// The list pages are read again in full, rather than confirmed by a probe,
// since the user may be after what the probe can't see, such as checks
// that were run again.
func (s *Service) Invalidate(repo core.RepoRef) {
	s.refreshes.set(repo, s.now())
	s.invalidate(repo)
}

// invalidate marks everything cached of repo stale, as Invalidate does,
// for a change a probe found, which a later probe may confirm.
func (s *Service) invalidate(repo core.RepoRef) {
	s.seen.DeletePrefix(pullPrefix(repo))
	tag := repoTag(repo)
	s.lists.InvalidateTag(tag)
	s.details.InvalidateTag(tag)
	s.comments.InvalidateTag(tag)
	s.reviews.InvalidateTag(tag)
}
