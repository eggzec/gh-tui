package repos

import "time"

// Option configures a Service.
type Option func(*options)

type options struct {
	ttl      time.Duration
	capacity int
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
