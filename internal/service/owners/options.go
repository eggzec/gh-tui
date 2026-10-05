package owners

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
)

// Option configures a Service.
type Option func(*options)

type options struct {
	ttls     TTLs
	sizes    Sizes
	capacity int
	store    cache.Store
}

// TTLs are how long each read of the service stays fresh before a read
// fetches it again. One that is zero or below is the default of the
// config (config.Default).
type TTLs struct {
	// Header covers an account's profile, counts and pins, which change
	// seldom.
	Header time.Duration
	// Repos covers the pages of an account's repositories.
	Repos time.Duration
	// Contributions covers a user's calendar, which only counts whole
	// days.
	Contributions time.Duration
	// People covers the lists of people of an account: followers,
	// following, organizations, members and teams.
	People time.Duration
	// Readme covers the profile README of an account.
	Readme time.Duration
}

// WithTTLs sets how long each read stays fresh.
func WithTTLs(t TTLs) Option {
	return func(o *options) { o.ttls = t }
}

// Sizes are how many items a page holds whose query sets no size, by what
// it lists. One that is zero or below is the default of the config
// (config.Default).
type Sizes struct {
	Repos  int
	People int
}

// WithSizes sets the size of the pages whose query sets none.
func WithSizes(s Sizes) Option {
	return func(o *options) {
		o.sizes = Sizes{Repos: max(s.Repos, 0), People: max(s.People, 0)}
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
// that a later session paints a page at once and fetches it again once
// read again. The store must be the signed-in account's alone. By default
// nothing outlives the service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}
