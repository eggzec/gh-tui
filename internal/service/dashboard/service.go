// Package dashboard serves what the dashboard shows from a cache: the
// viewer's profile with their pins and organizations, the work waiting on
// them, their contribution calendar, and the repositories of each owner
// they can switch to.
package dashboard

import (
	"cmp"
	"context"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/fallback"
)

// API is the part of the GitHub client the service uses.
type API interface {
	ViewerHeader(ctx context.Context) (core.Header, error)
	ViewerWork(ctx context.Context, first int) (core.Work, error)
	ViewerContributions(ctx context.Context) (core.Contributions, error)
	ViewerOwnRepos(ctx context.Context, first int, after string) (core.Page[core.Repo], error)
	OrgRepos(ctx context.Context, login string, first int, after string) (core.Page[core.Repo], error)
}

// How long the reads that change slowly stay fresh, unless the TTL of the
// service is longer. A profile, its pins and organizations change seldom,
// and the calendar only counts whole days.
const (
	HeaderTTL        = time.Hour
	ReposTTL         = 15 * time.Minute
	ContributionsTTL = 6 * time.Hour
)

// Service reads the dashboard through a cache. It is safe for concurrent
// use.
type Service struct {
	api           API
	header        reads[core.Header]
	work          reads[core.Work]
	contributions reads[core.Contributions]
	repos         reads[core.Page[core.Repo]]
	// workSize is how many items of each list of work a query that sets
	// none holds.
	workSize int
}

// The kinds of entries the service keeps in its store, and the version of
// their values. Bump schema when a core type they hold changes shape.
const (
	kindHeader        = "dashheader"
	kindWork          = "dashwork"
	kindContributions = "dashcontrib"
	kindRepos         = "ownerrepos"
	schema            = 4
)

// New returns a Service that fetches from api.
func New(api API, opts ...Option) *Service {
	o := options{ttl: cache.DefaultTTL}
	for _, opt := range opts {
		opt(&o)
	}
	return &Service{
		api: api,
		header: newReads(o, kindHeader, max(o.ttl, HeaderTTL), func(h *core.Header) (*bool, *bool, *bool) {
			return &h.Stale, &h.Offline, &h.Limited
		}),
		work: newReads(o, kindWork, o.ttl, func(w *core.Work) (*bool, *bool, *bool) {
			return &w.Stale, &w.Offline, &w.Limited
		}),
		contributions: newReads(o, kindContributions, max(o.ttl, ContributionsTTL), func(c *core.Contributions) (*bool, *bool, *bool) {
			return &c.Stale, &c.Offline, &c.Limited
		}),
		repos: newReads(o, kindRepos, max(o.ttl, ReposTTL), func(p *core.Page[core.Repo]) (*bool, *bool, *bool) {
			return &p.Stale, &p.Offline, &p.Limited
		}),
		workSize: cmp.Or(o.workSize, config.Default().PageSize.WaitingOnYou),
	}
}

// Invalidate marks everything the service cached stale. It is still served
// by the Cached reads, and the next read of each entry goes to GitHub, so a
// refresh reaches the server even while the entries are fresh.
func (s *Service) Invalidate() {
	s.header.mem.InvalidateTag(allTag)
	s.work.mem.InvalidateTag(allTag)
	s.contributions.mem.InvalidateTag(allTag)
	s.repos.mem.InvalidateTag(allTag)
}

// allTag marks every entry, so that Invalidate finds them all.
const allTag = "all"

// reads is one kind of read: its cache in memory, its shelf in the store,
// and how to mark a value served stale, offline or limited.
type reads[V any] struct {
	mem   *cache.Cache[V]
	kept  *cache.Shelf[V]
	flags func(*V) (stale, offline, limited *bool)
}

func newReads[V any](o options, kind string, ttl time.Duration, flags func(*V) (stale, offline, limited *bool)) reads[V] {
	return reads[V]{
		mem:   cache.New[V](cache.WithTTL(ttl), cache.WithCapacity(o.capacity)),
		kept:  cache.NewShelf[V](o.store, kind, schema),
		flags: flags,
	}
}

// cached returns the value under key in memory, fresh or stale, without
// I/O.
func (r *reads[V]) cached(key string) (V, bool) {
	e, state := r.mem.Get(key)
	return e.Value, state != cache.Miss
}

// fresh reports whether the value under key is in memory and fresh, so
// that reading it costs no request, without I/O.
func (r *reads[V]) fresh(key string) bool {
	_, state := r.mem.Get(key)
	return state == cache.Fresh
}

// get returns the value under key. A fresh value in memory is returned
// without a request, and so is one kept by an earlier session within the
// TTL. An older kept one is returned at once, marked stale, until a read
// with again set fetches it. Otherwise get fetches the value, stores it
// and keeps it, falling back on the stale value as fallback.Fetch does.
func (r *reads[V]) get(ctx context.Context, key string, again bool, fetch func(context.Context) (V, error)) (V, error) {
	if e, ok := r.kept.Warm(r.mem, key, again); ok {
		v := e.Value
		stale, _, _ := r.flags(&v)
		*stale = true
		return v, nil
	}
	// GraphQL has no validators, so a stale value is fetched again in full.
	e, err := fallback.Fetch(ctx, r.mem, r.kept, key, r.marks, fallback.Keep(r.kept, key, func(ctx context.Context, _ cache.Entry[V], _ bool) (cache.Entry[V], error) {
		v, err := fetch(ctx)
		if err != nil {
			return cache.Entry[V]{}, err
		}
		return cache.Entry[V]{Value: v, Tags: []string{allTag}}, nil
	}))
	return e.Value, err
}

// marks are the fallback.Marks of the values.
func (r *reads[V]) marks(v *V) (offline, limited *bool) {
	_, offline, limited = r.flags(v)
	return offline, limited
}
