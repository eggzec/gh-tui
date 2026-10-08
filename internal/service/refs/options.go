package refs

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
	pageSize int
	host     string
}

// WithTTL sets how long what was read of the links stays fresh. Without
// it, or with d at or below zero, it is the default of the config
// (config.Default).
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}

// WithCapacity sets how many entries each of the service's caches holds.
// Without it, or with n below one, it is the default of the config
// (config.Default).
func WithCapacity(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.capacity = n
		}
	}
}

// WithStore keeps what the service reads in store as well as in memory, so
// that a later session shows it at once. The store must be the signed-in
// account's alone. By default nothing outlives the service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}

// WithPageSize sets how many items a page of mentions holds when its
// query sets no size. Without it, or with n below one, it is the default
// of the config (config.Default).
func WithPageSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.pageSize = n
		}
	}
}

// WithHost sets the host of the session, which says what links in a text
// point to an item: only those to this host. By default it is github.com.
func WithHost(host string) Option {
	return func(o *options) { o.host = host }
}
