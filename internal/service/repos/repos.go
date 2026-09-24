// Package repos serves the viewer's repositories from a cache and stars them
// optimistically.
package repos

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// DefaultPageSize is the page size of a ListQuery that sets none.
const DefaultPageSize = 30

// maxPageSize is the largest page GitHub returns.
const maxPageSize = 100

// API is the part of the GitHub client the service uses.
type API interface {
	ListRepos(ctx context.Context, first int, after string) (core.Page[core.Repo], error)
	GetRepo(ctx context.Context, ref core.RepoRef) (core.Repo, error)
	Star(ctx context.Context, ref core.RepoRef) error
	Unstar(ctx context.Context, ref core.RepoRef) error
}

// ListQuery selects a page of the viewer's repositories.
type ListQuery struct {
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many repositories a page holds. Zero means
	// DefaultPageSize, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
}

func (q ListQuery) normalize() ListQuery {
	if q.PageSize <= 0 {
		q.PageSize = DefaultPageSize
	}
	q.PageSize = min(q.PageSize, maxPageSize)
	return q
}

// Service reads repositories through a cache. It is safe for concurrent use.
type Service struct {
	api   API
	lists *cache.Cache[core.Page[core.Repo]]
	repos *cache.Cache[core.Repo]
	// kept holds the list pages an earlier session read, if the service
	// has a store.
	kept *cache.Shelf[core.Page[core.Repo]]
}

// kind is what the service keeps its list pages as, and schema the version
// of core.Repo they hold. Bump it when the type changes shape.
const (
	kind   = "repolist"
	schema = 1
)

// offlineAt is when a page served offline was fetched, as far as the cache
// can tell: long ago, so it is stale at once and the next read asks GitHub
// again.
var offlineAt = time.Unix(1, 0)

// New returns a Service that fetches from api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	copts := []cache.Option{cache.WithTTL(o.ttl), cache.WithCapacity(o.capacity)}
	return &Service{
		api:   api,
		lists: cache.New[core.Page[core.Repo]](copts...),
		repos: cache.New[core.Repo](copts...),
		kept:  cache.NewShelf[core.Page[core.Repo]](o.store, kind, schema),
	}
}

// CachedList returns the cached page for q, fresh or stale, without I/O.
func (s *Service) CachedList(q ListQuery) (core.Page[core.Repo], bool) {
	e, state := s.lists.Get(listKey(q.normalize()))
	return e.Value, state != cache.Miss
}

// List returns the page for q. A fresh cached page is returned without a
// request.
//
// A page that only an earlier session kept is returned at once, with Stale
// set, and reading it again fetches it. If GitHub can't be reached, a stale
// page is served with Offline set.
func (s *Service) List(ctx context.Context, q ListQuery) (core.Page[core.Repo], error) {
	q = q.normalize()
	key := listKey(q)
	if e, ok := s.kept.Warm(s.lists, key); ok {
		p := e.Value
		p.Stale = true
		return p, nil
	}
	e, err := s.lists.Fetch(ctx, key, func(ctx context.Context, prev cache.Entry[core.Page[core.Repo]], ok bool) (cache.Entry[core.Page[core.Repo]], error) {
		// GraphQL has no validators, so a stale page is fetched again in full.
		p, err := s.api.ListRepos(ctx, q.PageSize, q.Cursor)
		switch {
		case ok && github.Unreachable(ctx, err):
			prev.Value.Offline, prev.FetchedAt = true, offlineAt
			return prev, nil
		case err != nil:
			if github.Refused(err) {
				s.kept.Delete(key)
			}
			return cache.Entry[core.Page[core.Repo]]{}, err
		}
		tags := make([]string, 0, len(p.Items)+1)
		tags = append(tags, allTag)
		for i := range p.Items {
			tags = append(tags, repoTag(p.Items[i].Ref))
		}
		e := cache.Entry[core.Page[core.Repo]]{Value: p, Tags: tags}
		// The shelf is only a shortcut, so a failure is ignored.
		_ = s.kept.Save(key, e)
		return e, nil
	})
	if err != nil {
		return core.Page[core.Repo]{}, fmt.Errorf("list repos: %w", err)
	}
	return e.Value, nil
}

// CachedGet returns the cached repository, fresh or stale, without I/O.
func (s *Service) CachedGet(ref core.RepoRef) (core.Repo, bool) {
	e, state := s.repos.Get(repoKey(ref))
	return e.Value, state != cache.Miss
}

// Get returns one repository. A fresh cached repository is returned without
// a request.
func (s *Service) Get(ctx context.Context, ref core.RepoRef) (core.Repo, error) {
	e, err := s.repos.Fetch(ctx, repoKey(ref), func(ctx context.Context, _ cache.Entry[core.Repo], _ bool) (cache.Entry[core.Repo], error) {
		r, err := s.api.GetRepo(ctx, ref)
		if err != nil {
			return cache.Entry[core.Repo]{}, err
		}
		return cache.Entry[core.Repo]{Value: r, Tags: []string{allTag, repoTag(ref)}}, nil
	})
	if err != nil {
		return core.Repo{}, fmt.Errorf("get repo %s: %w", ref, err)
	}
	return e.Value, nil
}

// Invalidate marks every cached list page and repository stale. They are
// still served by the Cached reads, and the next fetch of each goes to
// GitHub, so a refresh reaches the server even while the entries are fresh.
func (s *Service) Invalidate() {
	s.lists.InvalidateTag(allTag)
	s.repos.InvalidateTag(allTag)
}

// allTag marks every entry, so Invalidate finds pages that list no
// repository too.
const allTag = "all"

func listKey(q ListQuery) string {
	return "list?page_size=" + strconv.Itoa(q.PageSize) + "&cursor=" + q.Cursor
}

// GitHub matches owners and names without regard to case, so keys and tags
// do too.

func repoKey(ref core.RepoRef) string {
	return "repo/" + strings.ToLower(ref.String())
}

// repoTag marks every entry, list page or repository, that holds ref.
func repoTag(ref core.RepoRef) string {
	return "repo:" + strings.ToLower(ref.String())
}
