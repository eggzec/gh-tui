// Package releases reads the releases of GitHub repositories, which
// notifications name by ID, through a cache. A release seldom changes once
// published, so it stays fresh for long, and is then revalidated with its
// ETag, which costs no rate limit when it didn't change.
package releases

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

// API is the part of the GitHub client the service uses.
type API interface {
	GetRelease(ctx context.Context, repo core.RepoRef, id int64, cond github.Conditional) (core.Release, github.Response, error)
}

// DefaultTTL is how long a release read stays fresh by default.
const DefaultTTL = time.Hour

// Service reads releases. It is safe for concurrent use.
type Service struct {
	api   API
	cache *cache.Cache[core.Release]
	// kept holds what an earlier session read, if the service has a store.
	kept *cache.Shelf[core.Release]
}

// Option configures a Service.
type Option func(*options)

type options struct {
	cache []cache.Option
	store cache.Store
}

// WithTTL sets how long a release read stays fresh. The default is
// DefaultTTL.
func WithTTL(d time.Duration) Option {
	return func(o *options) { o.cache = append(o.cache, cache.WithTTL(d)) }
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
	schema = 1
)

// offlineAt is when a release served offline was fetched, as far as the
// cache can tell: long ago, so that the next read asks GitHub again.
var offlineAt = time.Unix(1, 0)

// New returns a service that reads releases through api.
func New(api API, opts ...Option) *Service {
	o := options{cache: []cache.Option{cache.WithTTL(DefaultTTL)}}
	for _, opt := range opts {
		opt(&o)
	}
	return &Service{
		api:   api,
		cache: cache.New[core.Release](o.cache...),
		kept:  cache.NewShelf[core.Release](o.store, kind, schema),
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
	return e.Value, st != cache.Miss
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
	s.kept.Warm(s.cache, k)
	e, err := s.cache.Fetch(ctx, k, func(ctx context.Context, prev cache.Entry[core.Release], ok bool) (cache.Entry[core.Release], error) {
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		r, res, err := s.api.GetRelease(ctx, repo, id, cond)
		switch {
		case ok && github.Unreachable(ctx, err):
			prev.FetchedAt = offlineAt
			return prev, nil
		case err != nil:
			if github.Refused(err) {
				s.kept.Delete(k)
			}
			return cache.Entry[core.Release]{}, err
		case res.NotModified:
			return cache.Entry[core.Release]{}, cache.ErrNotModified
		}
		e := cache.Entry[core.Release]{Value: r, ETag: res.ETag, LastModified: res.LastModified, Source: res.URL}
		// The shelf is only a shortcut, so a failure is ignored.
		_ = s.kept.Save(k, e)
		return e, nil
	})
	if err != nil {
		return core.Release{}, fmt.Errorf("get release %d of %s: %w", id, repo, err)
	}
	return e.Value, nil
}
