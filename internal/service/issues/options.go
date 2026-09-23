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
}

// WithTTL sets how long fetched issues count as fresh. Until then, reads
// make no request. The default is cache.DefaultTTL.
func WithTTL(d time.Duration) Option {
	return func(o *options) { o.cache = append(o.cache, cache.WithTTL(d)) }
}

// WithCapacity sets how many list pages, issues and comment threads are
// each kept. The default is cache.DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) { o.cache = append(o.cache, cache.WithCapacity(n)) }
}

// WithViewer sets the login of the signed-in user, who is shown as the
// author of a comment until GitHub confirms it.
func WithViewer(login string) Option {
	return func(o *options) { o.viewer = login }
}
