package files

import "time"

// Option configures a Service.
type Option func(*options)

type options struct {
	ttl          time.Duration
	capacity     int
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
// GitHub whether the ref moved. Trees and blobs read by SHA never go
// stale. Without it, or with d at or below zero, it is the default of the config (config.Default).
func WithTTL(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.ttl = d
		}
	}
}

// WithCapacity sets how many trees are kept, separately for trees read by a
// ref and by SHA. Without it, or with n below one, it is the default of the config (config.Default).
func WithCapacity(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.capacity = n
		}
	}
}

// WithBlobCapacity sets how many file contents are kept in memory. Without
// it, or with n below one, it is the default of the config (config.Default).
func WithBlobCapacity(n int) Option {
	return func(o *options) {
		if n >= 1 {
			o.blobCapacity = n
		}
	}
}

// WithBlobMemory sets the total size, in bytes, of the file contents kept in
// memory. Without it, or with n below one, it is the default of the config (config.Default).
func WithBlobMemory(n int64) Option {
	return func(o *options) {
		if n >= 1 {
			o.blobMemory = n
		}
	}
}

// WithMaxBlobSize sets the largest file, in bytes, that Blob reads. Larger
// files are better opened in the browser. Without it, or with n below one,
// it is the default of the config (config.Default).
func WithMaxBlobSize(n int64) Option {
	return func(o *options) {
		if n >= 1 {
			o.maxBlob = n
		}
	}
}
