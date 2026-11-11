// Package releases reads the releases of GitHub repositories, which
// notifications name by ID, through a cache. A release seldom changes once
// published, so it stays fresh for long, and is then revalidated with its
// ETag, which costs no rate limit when it didn't change.
package releases

import (
	"cmp"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/fallback"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// API is the part of the GitHub client the service uses.
type API interface {
	GetRelease(ctx context.Context, repo core.RepoRef, id int64, cond github.Conditional) (core.Release, github.Response, error)
	ListReleases(ctx context.Context, repo core.RepoRef, cursor string, perPage int, cond github.Conditional) (core.Page[core.Release], github.Response, error)
	LatestRelease(ctx context.Context, repo core.RepoRef) (core.Release, bool, error)
	ReleaseAssetURL(ctx context.Context, repo core.RepoRef, assetID int64) (string, error)
}

// Service reads releases. It is safe for concurrent use.
type Service struct {
	api   API
	ttl   time.Duration
	cache *cache.Cache[core.Release]
	// kept holds what an earlier session read, if the service has a store.
	kept *cache.Shelf[core.Release]

	lists     *cache.Cache[core.Page[core.Release]]
	keptLists *cache.Shelf[core.Page[core.Release]]
	pageSize  int

	// latest is the ID of each repository's latest release, by repoID, read
	// once and kept for the session. Pages are marked with it on the way out,
	// so what is cached never holds a mark that has gone stale.
	latestMu sync.Mutex
	latest   map[string]int64
}

// Option configures a Service.
type Option func(*options)

type options struct {
	ttl      time.Duration
	capacity int
	store    cache.Store
	pageSize int
}

// WithPageSize sets how many releases a page of a list holds. Without it,
// or with n below one, it is the default of the config (config.Default).
func WithPageSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.pageSize = n
		}
	}
}

// WithTTL sets how long a release read stays fresh. A release seldom
// changes once published, so it is usually long. Without it, or with d at
// or below zero, it is the default of the config (config.Default).
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}

// WithCapacity sets how many releases are kept in memory. Without it, or
// with n below one, it is the default of the config (config.Default).
func WithCapacity(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.capacity = n
		}
	}
}

// WithStore keeps the releases read in store as well as in memory, so that
// a later session shows them at once. The store must be the signed-in
// account's alone. By default nothing outlives the service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}

// kind is what the service keeps its releases as, and schema the version
// of core.Release they hold. Bump it when the type changes shape.
const (
	kind   = "release"
	schema = 2
	// kindList is what pages of releases are kept as.
	kindList = "releaselist"
)

// New returns a service that reads releases through api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := config.Default().Cache
	ttl := cmp.Or(o.ttl, d.TTL.Releases)
	capacity := cmp.Or(o.capacity, d.Memory.Entries)
	return &Service{
		api:       api,
		ttl:       ttl,
		cache:     cache.New[core.Release](cache.WithTTL(ttl), cache.WithCapacity(capacity)),
		kept:      cache.NewShelf[core.Release](o.store, kind, schema),
		lists:     cache.New[core.Page[core.Release]](cache.WithTTL(ttl), cache.WithCapacity(capacity)),
		keptLists: cache.NewShelf[core.Page[core.Release]](o.store, kindList, schema),
		pageSize:  cmp.Or(o.pageSize, config.Default().PageSize.Releases),
		latest:    map[string]int64{},
	}
}

// key names release id of repo. GitHub ignores the case of owners and
// names, so keys do too.
func key(repo core.RepoRef, id int64) string {
	return "release:" + strings.ToLower(repo.String()) + "/" + strconv.FormatInt(id, 10)
}

// CachedGet returns release id of repo if it is in memory, fresh or stale,
// without a request.
func (s *Service) CachedGet(repo core.RepoRef, id int64) (core.Release, bool) {
	e, st := s.cache.Get(key(repo, id))
	return s.mark(repo, e.Value), st != cache.Miss
}

// Current reports whether release id of repo is in memory and fresh, so
// that Get returns it without a request. It does no I/O.
func (s *Service) Current(repo core.RepoRef, id int64) bool {
	_, st := s.cache.Get(key(repo, id))
	return st == cache.Fresh
}

// Get returns release id of repo. A fresh release comes from the cache;
// otherwise it is fetched, conditionally if a stale copy is cached. What an
// earlier session kept counts as cached, and is served if GitHub can't be
// reached.
func (s *Service) Get(ctx context.Context, repo core.RepoRef, id int64) (core.Release, error) {
	k := key(repo, id)
	s.kept.Warm(s.cache, k, true)
	load := recheck.Load(func(ctx context.Context, cond github.Conditional) (core.Release, github.Response, error) {
		return s.api.GetRelease(ctx, repo, id, cond)
	}, func(core.Release) []string { return nil })
	e, err := fallback.Fetch(ctx, s.cache, s.kept, k, fallback.None[core.Release], fallback.Keep(s.kept, k, load))
	if err != nil {
		return core.Release{}, fmt.Errorf("get release %d of %s: %w", id, repo, err)
	}
	return s.mark(repo, e.Value), nil
}
