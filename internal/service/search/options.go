package search

import "time"

// Option configures a Service.
type Option func(*options)

type options struct {
	ttl, codeTTL time.Duration
	capacity     int
	pageSize     int
}

// WithPageSize sets how many results of each kind, or files, a page holds
// whose query sets no size, at most 100. By default, and for n below one, it
// is the default of the config (config.Default).
func WithPageSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.pageSize = n
		}
	}
}

// WithTTL sets how long a page of repositories, issues or pull requests,
// and the counts, stay fresh before a search asks GitHub again. Results
// change, and a query typed again soon after is the case the cache is
// for, so it is usually short. Without it, or with d at or below zero, it
// is the default of the config (config.Default).
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}

// WithCodeTTL sets how long a page of code search results stays fresh.
// Code search allows only 10 requests a minute, and GitHub indexes code
// with a delay anyway, so it is usually longer than the TTL. Without it,
// or with d at or below zero, it is the default of the config
// (config.Default).
func WithCodeTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.codeTTL = d
		}
	}
}

// WithCapacity sets how many pages of results of each search the service
// keeps. Without it, or with n below one, it is the default of the config
// (config.Default).
func WithCapacity(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.capacity = n
		}
	}
}
