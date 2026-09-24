package issues

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
)

// Option configures a Service.
type Option func(*options)

type options struct {
	cache  []cache.Option
	viewer string
	store  cache.Store
}

// WithTTL sets how long fetched issues count as fresh. Until then, reads
// make no request. The default is cache.DefaultTTL.
func WithTTL(d time.Duration) Option {
	return func(o *options) { o.cache = append(o.cache, cache.WithTTL(d)) }
}

// WithCapacity sets how many list pages, issues and comment pages are each
// kept. The default is cache.DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) { o.cache = append(o.cache, cache.WithCapacity(n)) }
}

// WithViewer sets the login of the signed-in user, who is shown as the
// author of a comment until GitHub confirms it.
func WithViewer(login string) Option {
	return func(o *options) { o.viewer = login }
}

// WithStore keeps list pages, issues and comment pages in store as well as
// in memory, so that a later session shows them at once and revalidates
// them with their validators. The store must be the signed-in account's
// alone. By default nothing outlives the service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}
