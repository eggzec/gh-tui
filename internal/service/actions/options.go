package actions

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
)

// Defaults used when no option overrides them.
const (
	// DefaultLiveTTL is how long the jobs of a run in progress stay fresh:
	// their steps move by the second, and a revalidation is free.
	DefaultLiveTTL = 10 * time.Second
	// DefaultLogMemory bounds the memory that parsed logs take.
	DefaultLogMemory = 64 << 20
)

// Option configures a Service.
type Option func(*options)

type options struct {
	ttl       time.Duration
	liveTTL   time.Duration
	capacity  int
	logMemory int64
	logLimit  int64
	store     cache.Store
	access    Access
}

// WithTTL sets how long runs, workflows, checks and annotations stay fresh
// before a read asks GitHub whether they changed. The default is
// cache.DefaultTTL.
func WithTTL(d time.Duration) Option {
	return func(o *options) { o.ttl = d }
}

// WithLiveTTL sets how long the jobs of a run that may still change stay
// fresh. The default is DefaultLiveTTL. Values below or equal to zero are
// ignored.
func WithLiveTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.liveTTL = d
		}
	}
}

// WithCapacity sets how many entries each of the service's caches keeps,
// apart from logs, which WithLogMemory bounds. The default is
// cache.DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) { o.capacity = n }
}

// WithLogMemory sets the memory, in bytes, that parsed logs take. The
// default is DefaultLogMemory. Values below 1 are ignored.
func WithLogMemory(n int64) Option {
	return func(o *options) {
		if n >= 1 {
			o.logMemory = n
		}
	}
}

// WithLogLimit sets how much of a log is read at most, in bytes: a larger
// one is read from its end. The default is github.DefaultLogLimit. Values
// below 1 are ignored.
func WithLogLimit(n int64) Option {
	return func(o *options) {
		if n >= 1 {
			o.logLimit = n
		}
	}
}

// WithStore keeps the first pages of runs and the workflows in store, so
// that a later session shows them at once and revalidates them with their
// validators, and keeps there for good what can't change: the jobs of
// attempts that completed and the logs of jobs that completed. The store
// must be the signed-in account's alone, since logs of private
// repositories are kept by job ID. By default nothing outlives the
// service.
func WithStore(store cache.Store) Option {
	return func(o *options) { o.store = store }
}

// WithAccess has the service ask access before it re-runs or cancels a
// run, so that one the token may not make is neither shown nor sent. By
// default every change is sent, and GitHub has the last word.
func WithAccess(access Access) Option {
	return func(o *options) { o.access = access }
}
