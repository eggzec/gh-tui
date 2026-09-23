package cache

import "time"

// Defaults used when no option overrides them.
const (
	DefaultCapacity = 1024
	DefaultTTL      = time.Minute
)

type options struct {
	capacity int
	ttl      time.Duration
}

// Option configures a Cache.
type Option func(*options)

// WithCapacity sets the maximum number of entries. When the cache is full,
// Set evicts the least recently used entry. Values below 1 are ignored.
func WithCapacity(n int) Option {
	return func(o *options) {
		if n >= 1 {
			o.capacity = n
		}
	}
}

// WithTTL sets how long an entry stays fresh after it was fetched. Values
// below or equal to zero are ignored.
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}
