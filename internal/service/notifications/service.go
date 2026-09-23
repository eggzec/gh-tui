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
	ListNotifications(ctx context.Context, filter core.NotificationFilter, cursor string, cond github.Conditional) (core.Page[core.Notification], github.Response, error)
	MarkThreadRead(ctx context.Context, id string) error
	MarkThreadDone(ctx context.Context, id string) error
	MarkNotificationsRead(ctx context.Context, lastReadAt time.Time) error
}

// Service serves notifications. It is safe for concurrent use.
type Service struct {
	api   API
	cache *cache.Cache[page]
	now   func() time.Time
	// interval is the latest X-Poll-Interval, in nanoseconds.
	interval atomic.Int64
}

type page = core.Page[core.Notification]

// Option configures a Service.
type Option func(*options)

type options struct {
	cache []cache.Option
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

// New returns a service that reads and writes notifications through api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return &Service{api: api, cache: cache.New[page](o.cache...), now: time.Now}
}
