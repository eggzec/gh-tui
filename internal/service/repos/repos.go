// Package repos serves the viewer's repositories from a cache and stars them
// optimistically.
package repos

import (
	"cmp"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/fallback"
)

// maxPageSize is the largest page GitHub returns.
const maxPageSize = 100

// API is the part of the GitHub client the service uses.
type API interface {
	ListRepos(ctx context.Context, first int, after string) (core.Page[core.Repo], error)
	GetRepo(ctx context.Context, ref core.RepoRef) (core.Repo, error)
	Star(ctx context.Context, ref core.RepoRef) error
	Unstar(ctx context.Context, ref core.RepoRef) error
}

// Access tells whether the token may do what an operation needs, as the
// access service does: nil, or why not.
type Access interface {
	Check(n core.Need) error
}

// ListQuery selects a page of the viewer's repositories.
type ListQuery struct {
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many repositories a page holds. Zero means the
	// service's page size, and sizes above GitHub's maximum of 100 are
	// clamped.
	PageSize int
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

// normalize returns q with its defaults set: size is the service's page
// size.
func (q ListQuery) normalize(size int) ListQuery {
	if q.PageSize <= 0 {
		q.PageSize = size
	}
	q.PageSize = min(q.PageSize, maxPageSize)
	return q
}

// Service reads repositories through a cache. It is safe for concurrent use.
type Service struct {
	api API
	// access refuses a star the token may not change before it is shown,
	// if set.
	access Access
	lists  *cache.Cache[core.Page[core.Repo]]
	repos  *cache.Cache[core.Repo]
	// kept holds the list pages an earlier session read, and keptRepos
	// the repositories, if the service has a store.
	kept      *cache.Shelf[core.Page[core.Repo]]
	keptRepos *cache.Shelf[core.Repo]
	// pageSize is the size of a page whose query sets none.
	pageSize int
}

// kind is what the service keeps its list pages as, and kindRepo its
// repositories; schema is the version of core.Repo they hold. Bump it when
// the type changes shape.
const (
	kind     = "repolist"
	kindRepo = "repo"
	schema   = 3
)

// New returns a Service that fetches from api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := config.Default()
	capacity := cache.WithCapacity(cmp.Or(o.capacity, d.Cache.Memory.Entries))
	return &Service{
		api:       api,
		access:    o.access,
		lists:     cache.New[core.Page[core.Repo]](cache.WithTTL(cmp.Or(o.ttl, d.Cache.TTL.Repos)), capacity),
		repos:     cache.New[core.Repo](cache.WithTTL(cmp.Or(o.infoTTL, d.Cache.TTL.RepoInfo)), capacity),
		kept:      cache.NewShelf[core.Page[core.Repo]](o.store, kind, schema),
		keptRepos: cache.NewShelf[core.Repo](o.store, kindRepo, schema),
		pageSize:  cmp.Or(o.pageSize, d.PageSize.Repos),
	}
}

// CachedList returns the cached page for q, fresh or stale, without I/O.
func (s *Service) CachedList(q ListQuery) (core.Page[core.Repo], bool) {
	e, state := s.lists.Get(listKey(q.normalize(s.pageSize)))
	return e.Value, state != cache.Miss
}

// List returns the page for q. A fresh cached page is returned without a
// request.
//
// A page that only an earlier session kept is fresh if it was fetched or
// revalidated within the TTL. An older one is returned at once, with Stale
// set, to every read until one with q.Again set fetches it. If GitHub can't
// be reached, a stale page is served with Offline set, and if it rate
// limits the read, with Limited set.
func (s *Service) List(ctx context.Context, q ListQuery) (core.Page[core.Repo], error) {
	q = q.normalize(s.pageSize)
	key := listKey(q)
	if e, ok := s.kept.Warm(s.lists, key, q.Again); ok {
		p := e.Value
		p.Stale = true
		return p, nil
	}
	// GraphQL has no validators, so a stale page is fetched again in full.
	e, err := fallback.Fetch(ctx, s.lists, s.kept, key, fallback.Page[core.Repo], fallback.Keep(s.kept, key, func(ctx context.Context, _ cache.Entry[core.Page[core.Repo]], _ bool) (cache.Entry[core.Page[core.Repo]], error) {
		p, err := s.api.ListRepos(ctx, q.PageSize, q.Cursor)
		if err != nil {
			return cache.Entry[core.Page[core.Repo]]{}, err
		}
		tags := make([]string, 0, len(p.Items)+1)
		tags = append(tags, allTag)
		for i := range p.Items {
			tags = append(tags, repoTag(p.Items[i].Ref))
		}
		return cache.Entry[core.Page[core.Repo]]{Value: p, Tags: tags}, nil
	}))
	// The client already names the request in its error.
	return e.Value, err
}

// CachedGet returns the cached repository, fresh or stale, without I/O.
func (s *Service) CachedGet(ref core.RepoRef) (core.Repo, bool) {
	e, state := s.repos.Get(repoKey(ref))
	return e.Value, state != cache.Miss
}

// FreshGet reports whether the repository is cached and fresh in memory, so
// that Get returns it without a request. It does no I/O.
func (s *Service) FreshGet(ref core.RepoRef) bool {
	_, state := s.repos.Get(repoKey(ref))
	return state == cache.Fresh
}

// Get returns one repository, with what the viewer may do in it. A fresh
// cached repository is returned without a request, and so is one that an
// earlier session kept and fetched within its TTL (WithInfoTTL). An older
// kept one is fetched again, and served if GitHub can't be reached.
func (s *Service) Get(ctx context.Context, ref core.RepoRef) (core.Repo, error) {
	key := repoKey(ref)
	// A stale kept repository is only what to fall back on, as the header
	// and the gates read it once and would keep it.
	s.keptRepos.Warm(s.repos, key, true)
	e, err := fallback.Fetch(ctx, s.repos, s.keptRepos, key, fallback.None[core.Repo], fallback.Keep(s.keptRepos, key, func(ctx context.Context, _ cache.Entry[core.Repo], _ bool) (cache.Entry[core.Repo], error) {
		r, err := s.api.GetRepo(ctx, ref)
		if err != nil {
			return cache.Entry[core.Repo]{}, err
		}
		return cache.Entry[core.Repo]{Value: r, Tags: []string{allTag, repoTag(ref)}}, nil
	}))
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
