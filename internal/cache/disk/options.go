package disk

import "compress/gzip"

type options struct {
	maxSize int64
	level   int
	keep    []string
}

// Option configures a Store.
type Option func(*options)

// WithMaxSize sets the size, in bytes on disk, that Collect keeps the store
// under. Without it, or with a value below 1, Collect removes nothing.
func WithMaxSize(n int64) Option {
	return func(o *options) {
		if n >= 1 {
			o.maxSize = n
		}
	}
}

// WithCompression sets the gzip level that Put compresses objects with,
// such as gzip.BestSpeed or gzip.DefaultCompression, the default.
// gzip.NoCompression stores objects as they are. Values that aren't gzip
// levels are ignored. Objects are read whatever level they were written with.
func WithCompression(level int) Option {
	return func(o *options) {
		if level >= gzip.HuffmanOnly && level <= gzip.BestCompression {
			o.level = level
		}
	}
}

// WithKeep names files that Collect never removes, wherever they are in
// the store, such as a file kept beside the objects that isn't a cache.
// They don't count towards the size either.
func WithKeep(names ...string) Option {
	return func(o *options) { o.keep = append(o.keep, names...) }
}
