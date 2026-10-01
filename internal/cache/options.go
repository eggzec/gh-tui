package cache

import "time"

type options struct {
	capacity int
	ttl      time.Duration
	// maxSize bounds the total of size over the entries, when size is set.
	// size holds a func(V) int64, which New takes only for its own V.
	maxSize int64
	size    any
}

// Option configures a Cache.
type Option func(*options)

// WithCapacity sets the maximum number of entries. When the cache is full,
// Set evicts the least recently used entry. Without it, or with a value
// below 1, the number of entries has no bound.
func WithCapacity(n int) Option {
	return func(o *options) {
		if n >= 1 {
			o.capacity = n
		}
	}
}

// WithTTL sets how long an entry stays fresh after it was fetched. Without
// it, or with a value below or equal to zero, an entry stays fresh until
// it is marked stale or replaced.
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}

// WithMaxSize bounds the entries by their total size, as measured by size,
// on top of the capacity: Set evicts the least recently used entries until
// the rest fit, but always keeps the entry just set. Use it when entries vary
// a lot in size, such as file contents. The option applies only to a
// Cache[V] of the same V. Values of limit below 1 are ignored.
func WithMaxSize[V any](limit int64, size func(V) int64) Option {
	return func(o *options) {
		if limit >= 1 && size != nil {
			o.maxSize, o.size = limit, size
		}
	}
}
