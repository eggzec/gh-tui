package pulls

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/github"
)

// Option configures a Service.
type Option func(*options)

type options struct {
	store    cache.Store
	ttl      time.Duration
	capacity int
	// pageSize is that of a page whose query sets none.
	pageSize int
	// detailSizes are the sizes of the lists a detail reads.
	detailSizes github.DetailSizes
	// diffMemory bounds the memory of the cached pages of changed files.
	diffMemory int64
	access     Access
	repos      Repos
}

// WithTTL sets how long fetched pull requests stay fresh. Without it, or
// with d at or below zero, it is the default of the config (config.Default).
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}

// WithPageSize sets how many pull requests, comments or reviews a page
// holds whose query sets no size, at most 100. By default, and for n below
// one, it is the default of the config (config.Default).
func WithPageSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.pageSize = n
		}
	}
}

// WithDetailSizes sets how many review threads, review requests and latest
// reviews, and rules of rulesets the detail of a pull request reads, each
// at most 100. Each size that is below one is the default of the config
// (config.Default).
func WithDetailSizes(z github.DetailSizes) Option {
	return func(o *options) {
		o.detailSizes = github.DetailSizes{Threads: min(max(z.Threads, 0), maxPageSize), Reviewers: min(max(z.Reviewers, 0), maxPageSize), Rules: min(max(z.Rules, 0), maxPageSize)}
	}
}

// WithCapacity sets how many entries each of the service's caches keeps:
// list pages, pull request details, comment pages and review pages.
// Without it, or with n below one, it is the default of the config (config.Default).
func WithCapacity(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.capacity = n
		}
	}
}

// WithDiffMemory sets the memory, in bytes, that the cached pages of the
// files a pull request changes take: a large change's patches may take
// megabytes. Without it, or with n below one, it is the default of the
// config (config.Default).
func WithDiffMemory(n int64) Option {
	return func(o *options) {
		if n >= 1 {
			o.diffMemory = n
		}
	}
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
