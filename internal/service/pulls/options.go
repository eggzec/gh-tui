package pulls

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
)

// Option configures a Service.
type Option func(*options)

type options struct {
	cache  []cache.Option
	store  cache.Store
	ttl    time.Duration
	access Access
	repos  Repos
}

// WithTTL sets how long fetched pull requests stay fresh. The default is
// cache.DefaultTTL.
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		o.cache = append(o.cache, cache.WithTTL(d))
		o.ttl = d
	}
}

// WithCapacity sets how many entries each of the service's caches keeps:
// list pages, pull request details, comment pages and review pages. The
// default is cache.DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) { o.cache = append(o.cache, cache.WithCapacity(n)) }
}

// WithStore keeps list pages, details and comment pages in store as well as
// in memory, so that a later session shows them at once and refetches them
// in the background. The store must be the signed-in account's alone. By
// default nothing outlives the service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}

// WithAccess has the service ask access before it changes a pull request,
// so that a change the token may not make is neither shown nor sent. By
// default every change is sent, and GitHub has the last word.
func WithAccess(access Access) Option {
	return func(o *options) { o.access = access }
}

// WithRepos lets the service tell a private repository from a public one
// by what repos holds of it, which a change there needs a wider scope for.
// A repository it holds nothing of may be either.
func WithRepos(repos Repos) Option {
	return func(o *options) { o.repos = repos }
}
