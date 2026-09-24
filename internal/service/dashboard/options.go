package dashboard

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

// WithTTL sets how long the work waiting stays fresh before a read fetches
// it again. By default it is cache.DefaultTTL. The other reads change less
// often and stay fresh for HeaderTTL, ReposTTL and ContributionsTTL, or for
// d if it is longer.
func WithTTL(d time.Duration) Option {
	return func(o *options) { o.ttl = d }
}

// WithCapacity sets how many entries each of the service's caches holds,
// most of them pages of repositories. By default it is
// cache.DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) { o.capacity = n }
}

// WithStore keeps what the service reads in store as well as in memory, so
// that a later session paints the dashboard at once and fetches it again
// once read again. The store must be the signed-in account's alone. By
// default nothing outlives the service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}
