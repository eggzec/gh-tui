// Package pulls serves pull requests from a cache in front of the GitHub
// API, and applies changes to them optimistically.
package pulls

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// API is the part of the GitHub client the service uses. The mutations take
// the node ID of the pull request and return it as the server left it.
type API interface {
	ListPullRequests(ctx context.Context, repo core.RepoRef, state core.State, cursor string, first int) (core.Page[core.PullRequest], error)
	GetPullRequest(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error)
	ListPullRequestComments(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.Comment], error)
	ListPullRequestReviews(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.Review], error)
	PullRequestID(ctx context.Context, repo core.RepoRef, number int) (string, error)
	MergePullRequest(ctx context.Context, id string, method core.MergeMethod) (core.PullRequest, error)
	ClosePullRequest(ctx context.Context, id string) (core.PullRequest, error)
	ReopenPullRequest(ctx context.Context, id string) (core.PullRequest, error)
	MarkPullRequestReady(ctx context.Context, id string) (core.PullRequest, error)
	ConvertPullRequestToDraft(ctx context.Context, id string) (core.PullRequest, error)
}

// Service reads pull requests through a cache and changes them
// optimistically. It is safe for concurrent use.
type Service struct {
	api     API
	lists   *cache.Cache[core.Page[core.PullRequest]]
	details *cache.Cache[core.PullRequestDetail]
	// Comments and reviews are cached a page at a time, so that the pages
	// the tui no longer shows are evicted.
	comments *cache.Cache[core.Page[core.Comment]]
	reviews  *cache.Cache[core.Page[core.Review]]
	now      func() time.Time
}

// New returns a service that reads from api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return &Service{
		api:      api,
		lists:    cache.New[core.Page[core.PullRequest]](o.cache...),
		details:  cache.New[core.PullRequestDetail](o.cache...),
		comments: cache.New[core.Page[core.Comment]](o.cache...),
		reviews:  cache.New[core.Page[core.Review]](o.cache...),
		now:      time.Now,
	}
}

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
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many pull requests the page holds at most. Zero means
	// 30, and GitHub's maximum of 100 caps it.
	PageSize int
}

func (q ListQuery) key() string {
	v := url.Values{"state": {string(q.State)}, "cursor": {q.Cursor}, "first": {strconv.Itoa(pageSize(q.PageSize))}}
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

func detailKey(r core.RepoRef, number int) string {
	return "pull:" + repoID(r) + "#" + strconv.Itoa(number)
}

// cached returns the value under key in c, fresh or stale, without
// fetching it.
func cached[V any](c *cache.Cache[V], key string) (V, bool) {
	e, state := c.Get(key)
	return e.Value, state != cache.Miss
}

// fetch returns the value under key in c. A fresh value is returned without
// a request; otherwise get fetches it, and it is stored tagged with repo.
func fetch[V any](ctx context.Context, c *cache.Cache[V], key string, repo core.RepoRef, get func(context.Context) (V, error)) (V, error) {
	// GraphQL responses carry no validators, so a stale value is fetched
	// again in full.
	e, err := c.Fetch(ctx, key, func(ctx context.Context, _ cache.Entry[V], _ bool) (cache.Entry[V], error) {
		v, err := get(ctx)
		if err != nil {
			return cache.Entry[V]{}, err
		}
		return cache.Entry[V]{Value: v, Tags: []string{repoTag(repo)}}, nil
	})
	return e.Value, err
}

// CachedList returns the page for q if it is cached, fresh or stale, without
// fetching it.
func (s *Service) CachedList(q ListQuery) (core.Page[core.PullRequest], bool) {
	return cached(s.lists, q.key())
}

// List returns the page for q, most recently updated first. A fresh cached
// page is returned without a request.
func (s *Service) List(ctx context.Context, q ListQuery) (core.Page[core.PullRequest], error) {
	p, err := fetch(ctx, s.lists, q.key(), q.Repo, func(ctx context.Context) (core.Page[core.PullRequest], error) {
		return s.api.ListPullRequests(ctx, q.Repo, q.State, q.Cursor, pageSize(q.PageSize))
	})
	if err != nil {
		return core.Page[core.PullRequest]{}, fmt.Errorf("list pulls of %s: %w", q.Repo, err)
	}
	return p, nil
}

// CachedGet returns pull request number of repo if its detail is cached,
// fresh or stale, without fetching it.
func (s *Service) CachedGet(repo core.RepoRef, number int) (core.PullRequestDetail, bool) {
	return cached(s.details, detailKey(repo, number))
}

// Get returns pull request number of repo with its body and checks. Its
// comments and reviews are read with Comments and Reviews. A fresh cached
// detail is returned without a request.
func (s *Service) Get(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error) {
	d, err := fetch(ctx, s.details, detailKey(repo, number), repo, func(ctx context.Context) (core.PullRequestDetail, error) {
		return s.api.GetPullRequest(ctx, repo, number)
	})
	if err != nil {
		return core.PullRequestDetail{}, fmt.Errorf("get pull %s#%d: %w", repo, number, err)
	}
	return d, nil
}

// Invalidate marks everything cached of repo stale: its list pages, details,
// comments and reviews. They are still served by the Cached reads, and the
// next fetch of each goes to GitHub, so a refresh reaches the server even
// while the entries are fresh.
func (s *Service) Invalidate(repo core.RepoRef) {
	tag := repoTag(repo)
	s.lists.InvalidateTag(tag)
	s.details.InvalidateTag(tag)
	s.comments.InvalidateTag(tag)
	s.reviews.InvalidateTag(tag)
}
