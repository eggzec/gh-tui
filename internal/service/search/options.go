package search

import "time"

// Option configures a Service.
type Option func(*options)

type options struct {
	ttl      time.Duration
	capacity int
}

// WithTTL sets how long a page of results stays fresh before a search asks
// GitHub again. By default it is DefaultTTL.
func WithTTL(d time.Duration) Option {
	return func(o *options) { o.ttl = d }
}

// WithCapacity sets how many pages of results the service keeps. By default
// it is DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) { o.capacity = n }
}
