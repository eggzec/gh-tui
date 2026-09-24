package files

import (
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
)

// Defaults used when no option overrides them.
const (
	// DefaultMaxBlobSize is the largest file the service reads, 1 MiB.
	// Larger files are better opened in the browser.
	DefaultMaxBlobSize = 1 << 20
	// DefaultBlobCapacity is how many file contents are kept. It is lower
	// than for trees since each one may be as large as the size limit.
	DefaultBlobCapacity = 64
)

// Option configures a Service.
type Option func(*options)

type options struct {
	cache        []cache.Option
	blobCapacity int
	maxBlob      int64
}

// WithTTL sets how long trees read by a ref stay fresh before a read asks
// GitHub whether the ref moved. The default is cache.DefaultTTL. Trees and
// blobs read by SHA never go stale.
func WithTTL(d time.Duration) Option {
	return func(o *options) { o.cache = append(o.cache, cache.WithTTL(d)) }
}

// WithCapacity sets how many trees are kept, separately for trees read by a
// ref and by SHA. The default is cache.DefaultCapacity.
func WithCapacity(n int) Option {
	return func(o *options) { o.cache = append(o.cache, cache.WithCapacity(n)) }
}

// WithBlobCapacity sets how many file contents are kept. The default is
// DefaultBlobCapacity. Values below 1 are ignored.
func WithBlobCapacity(n int) Option {
	return func(o *options) {
		if n >= 1 {
			o.blobCapacity = n
		}
	}
}

// WithMaxBlobSize sets the largest file, in bytes, that Blob reads. The
// default is DefaultMaxBlobSize. Values below 1 are ignored.
func WithMaxBlobSize(n int64) Option {
	return func(o *options) {
		if n >= 1 {
			o.maxBlob = n
		}
	}
}
