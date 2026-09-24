package history

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
)

// Defaults used when no option overrides them.
const (
	// DefaultCompareTTL is how long a comparison stays fresh. Refs move
	// more often than lists change, and a revalidation is free.
	DefaultCompareTTL = 30 * time.Second
	// DefaultDiffMemory bounds the memory that the changes of commits
	// take, separately for details and for later pages of files: a large
	// commit's patches may take megabytes.
	DefaultDiffMemory = 32 << 20
)

// Option configures a Service.
type Option func(*options)

type options struct {
	ttl        time.Duration
	compareTTL time.Duration
	capacity   int
	diffMemory int64
	store      cache.Store
	objects    cache.Store
}

// WithTTL sets how long branches and the first pages of commits of a ref
// stay fresh before a read asks GitHub whether they changed. The default
// is cache.DefaultTTL. What a SHA names never goes stale.
func WithTTL(d time.Duration) Option {
	return func(o *options) { o.ttl = d }
}

// WithCompareTTL sets how long a comparison stays fresh. The default is
// DefaultCompareTTL.
func WithCompareTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.compareTTL = d
		}
	}
}

// WithCapacity sets how many pages of branches, of commits and
// comparisons are each kept in memory. The default is
// cache.DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) { o.capacity = n }
}

// WithDiffMemory sets the memory, in bytes, that the changes of commits
// take. The default is DefaultDiffMemory. Values below 1 are ignored.
func WithDiffMemory(n int64) Option {
	return func(o *options) {
		if n >= 1 {
			o.diffMemory = n
		}
	}
}

// WithStore keeps the pages of branches and the first pages of commits of
// refs in store as well as in memory, so that a later session shows them
// at once and revalidates them with their validators. The store must be
// the signed-in account's alone. By default nothing outlives the service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}

// WithObjects keeps what a commit SHA names, which never changes, in store:
// the pages of commits after the first and the changes of commits. Like
// the objects of the files service, they are named by the repository and
// the SHA, so the store may be shared by accounts on a host. By default
// nothing outlives the service.
func WithObjects(store cache.Store) Option {
	return func(o *options) { o.objects = store }
}
