package dashboard

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
)

// Option configures a Service.
type Option func(*options)

type options struct {
	ttls     TTLs
	capacity int
	store    cache.Store
	workSize int
}

// WithWorkSize sets how many of the most recently updated items each list
// of work holds whose query sets no size, at most 100. By default, and for
// n below one, it is the default of the config (config.Default).
func WithWorkSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.workSize = n
		}
	}
}

// TTLs are how long each read of the service stays fresh before a read
// fetches it again. One that is zero or below is the default of the
// config (config.Default).
type TTLs struct {
	// Header covers the viewer, their organizations and their pins, which
	// change seldom.
	Header time.Duration
	// Work covers the work waiting on the viewer.
	Work time.Duration
	// Repos covers the pages of every owner's repositories, which cost
	// many requests to read again.
	Repos time.Duration
	// Contributions covers the calendar, which only counts whole days.
	Contributions time.Duration
}

// WithTTLs sets how long each read stays fresh.
func WithTTLs(t TTLs) Option {
	return func(o *options) { o.ttls = t }
}

// WithCapacity sets how many entries each of the service's caches holds,
// most of them pages of repositories. Without it, or with n below one, it
// is the default of the config (config.Default).
func WithCapacity(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.capacity = n
		}
	}
}

// WithStore keeps what the service reads in store as well as in memory, so
// that a later session paints the dashboard at once and fetches it again
// once read again. The store must be the signed-in account's alone. By
// default nothing outlives the service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}
