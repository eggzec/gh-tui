package core

import (
	"context"
	"sync/atomic"
)

type limitWatchKey struct{}

// WatchLimit returns ctx, under which a read that GitHub rate limits but
// that is served what was kept in its place says so (ServedLimited), and
// limited, which reports whether any read under it did. Such a read
// returns no error, only a value marked as served for the limit, and some
// values have no mark, so a caller that must stop at the rate limit, such
// as a read ahead, watches its reads with it.
func WatchLimit(ctx context.Context) (_ context.Context, limited func() bool) {
	seen := new(atomic.Bool)
	return context.WithValue(ctx, limitWatchKey{}, seen), seen.Load
}

// ServedLimited tells whoever watches ctx (WatchLimit) that a read under
// it was served what was kept, because GitHub rate limited it. Without a
// watch it does nothing.
func ServedLimited(ctx context.Context) {
	if seen, ok := ctx.Value(limitWatchKey{}).(*atomic.Bool); ok {
		seen.Store(true)
	}
}
