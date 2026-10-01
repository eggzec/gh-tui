package history

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
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
	// commitPageSize is that of a page of commits whose query sets none.
	commitPageSize int
}

// WithCommitPageSize sets how many commits a page holds whose query sets
// no size, at most 100. By default, and for n below one, it is
// the default of the config (config.Default).
func WithCommitPageSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.commitPageSize = n
		}
	}
}

// WithTTL sets how long branches and the first pages of commits of a ref
// stay fresh before a read asks GitHub whether they changed. What a SHA
// names never goes stale. Without it, or with d at or below zero, it is
// the default of the config (config.Default).
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}

// WithCompareTTL sets how long a comparison stays fresh. Refs move more
// often than lists change, and a revalidation is free, so it is usually
// shorter than the TTL. Without it, or with d at or below zero, it is the
// default of the config (config.Default).
func WithCompareTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.compareTTL = d
		}
	}
}

// WithCapacity sets how many pages of branches, of commits and of the
// files of commits, commits and comparisons are each kept in memory.
// WithDiffMemory bounds the commits and their files too. Without it, or
// with n below one, it is the default of the config (config.Default).
func WithCapacity(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.capacity = n
		}
	}
}

// WithDiffMemory sets the memory, in bytes, that the changes of commits
// take, separately for details and for later pages of files: a large
// commit's patches may take megabytes. Without it, or with n below one,
// it is the default of the config (config.Default).
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
