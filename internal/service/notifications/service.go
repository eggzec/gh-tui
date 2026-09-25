// Package notifications serves the user's inbox from a cache, keeps it in
// sync by polling, and marks threads read or done optimistically.
package notifications

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// API is the part of the GitHub client the service uses.
type API interface {
	ListNotifications(ctx context.Context, filter core.NotificationFilter, perPage int, cursor string, cond github.Conditional) (core.Page[core.Notification], github.Response, error)
	MarkThreadRead(ctx context.Context, id string) error
	MarkThreadDone(ctx context.Context, id string) error
	MarkNotificationsRead(ctx context.Context, lastReadAt time.Time) error
}

// Service serves notifications. It is safe for concurrent use.
type Service struct {
	api   API
	cache *cache.Cache[page]
	// kept holds what an earlier session read, if the service has a store.
	kept *cache.Shelf[page]
	now  func() time.Time
	// interval is the latest X-Poll-Interval, in nanoseconds.
	interval atomic.Int64
}

type page = core.Page[core.Notification]

// Option configures a Service.
type Option func(*options)

type options struct {
	cache []cache.Option
	store cache.Store
}

// WithTTL sets how long a fetched page stays fresh. The default is
// cache.DefaultTTL.
func WithTTL(d time.Duration) Option {
	return func(o *options) { o.cache = append(o.cache, cache.WithTTL(d)) }
}

// WithCapacity sets how many pages are cached. The default is
// cache.DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) { o.cache = append(o.cache, cache.WithCapacity(n)) }
}

// WithStore keeps the pages in store as well as in memory, so that a later
// session shows them at once and revalidates them with their validators.
// The store must be the signed-in account's alone. By default nothing
// outlives the service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}

// kind is what the service keeps its pages as, and schema the version of
// core.Notification they hold. Bump it when the type changes shape.
const (
	kind   = "notifications"
	schema = 2
)

// New returns a service that reads and writes notifications through api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return &Service{
		api:   api,
		cache: cache.New[page](o.cache...),
		kept:  cache.NewShelf[page](o.store, kind, schema),
		now:   time.Now,
	}
}
