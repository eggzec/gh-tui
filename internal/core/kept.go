package core

import (
	"context"
	"sync/atomic"
)

type limitWatchKey struct{}

// limitWatch is what a watch notes, and the watch it is nested in, if
// any, which is told too.
type limitWatch struct {
	seen   atomic.Bool
	parent *limitWatch
}

// WatchLimit returns ctx, under which a read that GitHub rate limits but
// that is served what was kept in its place says so (ServedLimited), and
// limited, which reports whether any read under it did. Such a read
// returns no error, only a value marked as served for the limit, and some
// values have no mark, so a caller that must stop at the rate limit, such
// as a read ahead, watches its reads with it. A watch within another
// tells the outer one too.
func WatchLimit(ctx context.Context) (_ context.Context, limited func() bool) {
	parent, _ := ctx.Value(limitWatchKey{}).(*limitWatch)
	w := &limitWatch{parent: parent}
	return context.WithValue(ctx, limitWatchKey{}, w), w.seen.Load
}

// ServedLimited tells whoever watches ctx (WatchLimit), and the watches
// it is within, that a read under it was served what was kept, because
// GitHub rate limited it. Without a watch it does nothing.
func ServedLimited(ctx context.Context) {
	w, _ := ctx.Value(limitWatchKey{}).(*limitWatch)
	for ; w != nil; w = w.parent {
		w.seen.Store(true)
	}
}
