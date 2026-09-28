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
// and the counts, stay fresh before a search asks GitHub again. By default
// it is DefaultTTL.
func WithTTL(d time.Duration) Option {
	return func(o *options) { o.ttl = d }
}

// WithCodeTTL sets how long a page of code search results stays fresh. By
// default it is DefaultCodeTTL.
func WithCodeTTL(d time.Duration) Option {
	return func(o *options) { o.codeTTL = d }
}

// WithCapacity sets how many pages of results of each search the service
// keeps. By default it is DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) { o.capacity = n }
}
