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
	// DefaultBlobCapacity is how many file contents are kept in memory:
	// enough for the files read ahead of the preview in a few
	// repositories. DefaultBlobMemory bounds their total size, since each
	// may be as large as the size limit, though most are small.
	DefaultBlobCapacity = 1024
	DefaultBlobMemory   = 32 << 20
)

// Option configures a Service.
type Option func(*options)

type options struct {
	cache        []cache.Option
	blobCapacity int
	blobMemory   int64
	maxBlob      int64
	store        Store
}

// WithStore keeps trees, file contents and where refs point in store as
// well as in memory, so a later session reads them from there. By default
// nothing outlives the service.
func WithStore(store Store) Option {
	return func(o *options) {
		if store != nil {
			o.store = store
		}
	}
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

// WithBlobCapacity sets how many file contents are kept in memory. The
// default is DefaultBlobCapacity. Values below 1 are ignored.
func WithBlobCapacity(n int) Option {
	return func(o *options) {
		if n >= 1 {
			o.blobCapacity = n
		}
	}
}

// WithBlobMemory sets the total size, in bytes, of the file contents kept in
// memory. The default is DefaultBlobMemory. Values below 1 are ignored.
func WithBlobMemory(n int64) Option {
	return func(o *options) {
		if n >= 1 {
			o.blobMemory = n
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
