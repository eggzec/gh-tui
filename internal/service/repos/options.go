package repos

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
)

// Option configures a Service.
type Option func(*options)

type options struct {
	ttl      time.Duration
	infoTTL  time.Duration
	capacity int
	store    cache.Store
	access   Access
	// pageSize is that of a page whose query sets none.
	pageSize int
}

// WithPageSize sets how many repositories a page holds whose query sets
// no size, at most 100. By default, and for n below one, it is
// the default of the config (config.Default).
func WithPageSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.pageSize = n
		}
	}
}

// WithTTL sets how long fetched list pages stay fresh before a read
// revalidates them. Without it, or with d at or below zero, it is the
// default of the config (config.Default).
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}

// WithInfoTTL sets how long a repository that Get read stays fresh. What
// it holds, such as the viewer's permission and the merge methods, seldom
// changes, so it is usually longer than the TTL of the lists. Without it,
// or with d at or below zero, it is the default of the config (config.Default).
func WithInfoTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.infoTTL = d
		}
	}
}

// WithCapacity sets how many list pages, and separately how many
// repositories, the service keeps. Without it, or with n below one, it is
// the default of the config (config.Default).
func WithCapacity(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.capacity = n
		}
	}
}

// WithStore keeps the list pages and the repositories that Get read in
// store as well as in memory, so that a later session shows them at once
// and fetches them again once they are stale.
// The store must be the signed-in account's alone. By default nothing
// outlives the service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}

// WithAccess has the service ask access before it stars a repository or
// removes a star, so that one the token may not change is neither shown
// nor sent. By default every change is sent, and GitHub has the last word.
func WithAccess(access Access) Option {
	return func(o *options) { o.access = access }
}
