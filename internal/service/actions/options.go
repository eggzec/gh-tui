package actions

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
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
	// runPageSize is that of a page of runs whose query sets none.
	runPageSize int
}

// WithRunPageSize sets how many runs a page holds whose query sets no
// size, at most 100. By default, and for n below one, it is
// the default of the config (config.Default).
func WithRunPageSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.runPageSize = n
		}
	}
}

// WithTTL sets how long runs, workflows, checks and annotations stay fresh
// before a read asks GitHub whether they changed. Without it, or with d
// at or below zero, it is the default of the config (config.Default).
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}

// WithLiveTTL sets how long the jobs of a run that may still change stay
// fresh: their steps move by the second, and a revalidation is free, so
// it is usually much shorter than the TTL. Without it, or with d at or
// below zero, it is the default of the config (config.Default).
func WithLiveTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.liveTTL = d
		}
	}
}

// WithCapacity sets how many entries each of the service's caches keeps.
// WithLogMemory bounds the logs too. Without it, or with n below one, it
// is the default of the config (config.Default).
func WithCapacity(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.capacity = n
		}
	}
}

// WithLogMemory sets the memory, in bytes, that parsed logs take. Without
// it, or with n below one, it is the default of the config (config.Default).
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
