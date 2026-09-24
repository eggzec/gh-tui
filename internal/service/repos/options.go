package repos

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
)

// Option configures a Service.
type Option func(*options)

type options struct {
	ttl      time.Duration
	capacity int
	store    cache.Store
}

// WithTTL sets how long fetched repositories stay fresh before a read
// revalidates them. By default it is cache.DefaultTTL.
func WithTTL(d time.Duration) Option {
	return func(o *options) { o.ttl = d }
}

// WithCapacity sets how many list pages, and separately how many
// repositories, the service keeps. By default it is cache.DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) { o.capacity = n }
}

// WithStore keeps the list pages in store as well as in memory, so that a
// later session shows them at once and fetches them again once read again.
// The store must be the signed-in account's alone. By default nothing
// outlives the service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}
