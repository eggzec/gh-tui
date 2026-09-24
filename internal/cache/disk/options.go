package disk

import "compress/gzip"

// DefaultMaxSize is the size, in bytes, that Collect keeps the store under
// by default: 512 MiB.
const DefaultMaxSize = 512 << 20

type options struct {
	maxSize int64
	level   int
}

// Option configures a Store.
type Option func(*options)

// WithMaxSize sets the size, in bytes on disk, that Collect keeps the store
// under. The default is DefaultMaxSize. Values below 1 are ignored.
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
