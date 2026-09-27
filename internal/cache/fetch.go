package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
)

// ErrNotModified is returned by a FetchFunc when the server reports that the
// previous entry is still current, for example with HTTP 304.
var ErrNotModified = errors.New("not modified")

// FetchFunc loads the value for a key. When ok is true, prev is the cached
// entry, possibly stale, whose ETag and LastModified can make the request
// conditional. Return ErrNotModified to keep prev, as confirmed, without
// its Fallback.
type FetchFunc[V any] func(ctx context.Context, prev Entry[V], ok bool) (Entry[V], error)

// flight is one in-progress call of a FetchFunc, shared by every Fetch that
// asks for the same key meanwhile.
type flight[V any] struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	// abandoned is set when every waiter left. Its result is dropped, since
	// a newer flight may already be running.
	abandoned bool

	// The entry when the flight started, and its version and that of its
	// latest write.
	prev             Entry[V]
	hadPrev          bool
	version, written uint64

	entry Entry[V]
	err   error
}

// Fetch returns the entry for key if it is fresh. Otherwise it calls fn and
// stores the result. Concurrent Fetches for one key share a single call of
// fn, which is canceled once every caller's context is done. Errors are
// returned but not cached.
//
// If the entry is written while fn runs, the cache keeps that newer write
// and the result of fn goes only to the callers. If it is only invalidated,
// the cache keeps the result, stale, so that the Cached reads show it and
// the next Fetch asks again.
func (c *Cache[V]) Fetch(ctx context.Context, key string, fn FetchFunc[V]) (Entry[V], error) {
	c.mu.Lock()
	n, ok := c.items[key]
	if ok && c.state(n) == Fresh {
		c.moveToFront(n)
		e := n.entry
		c.count(key, obs.MemoryHit)
		c.mu.Unlock()
		logFetch(ctx, key, "hit")
		return e, nil
	}
	f, running := c.flights[key]
	if !running {
		if err := ctx.Err(); err != nil {
			c.mu.Unlock()
			return Entry[V]{}, err
		}
		f = c.start(ctx, key, n, fn)
	}
	f.waiters++
	found := "miss"
	switch {
	case running:
		found = "shared"
		c.count(key, obs.Shared)
	case ok:
		found = "stale"
		c.count(key, obs.MemoryMiss)
	default:
		c.count(key, obs.MemoryMiss)
	}
	c.mu.Unlock()
	logFetch(ctx, key, found)

	select {
	case <-f.done:
		return f.entry, f.err
	case <-ctx.Done():
		c.leave(key, f)
		return Entry[V]{}, ctx.Err()
	}
}

// Hit counts a read of key that its caller served from what Get returned,
// without a Fetch, as a memory hit, such as an entry that is stale by its
// TTL but that the caller knows to be current.
func (c *Cache[V]) Hit(key string) {
	c.count(key, obs.MemoryHit)
}

// start runs fn for key in the background. n is the current node, or nil.
// c.mu must be held.
func (c *Cache[V]) start(ctx context.Context, key string, n *node[V], fn FetchFunc[V]) *flight[V] {
	// The call outlives the caller that started it when others are waiting,
	// so it keeps ctx's values but not its cancellation.
	fctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	f := &flight[V]{done: make(chan struct{}), cancel: cancel}
	if n != nil {
		f.prev, f.hadPrev, f.version, f.written = n.entry, true, n.version, n.written
	}
	if c.flights == nil {
		c.flights = make(map[string]*flight[V])
	}
	c.flights[key] = f
	go func() {
		e, err := fn(fctx, f.prev, f.hadPrev)
		c.finish(key, f, e, err)
	}()
	return f
}

// finish records the result of f and wakes its waiters.
func (c *Cache[V]) finish(key string, f *flight[V], e Entry[V], err error) {
	defer f.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	defer close(f.done)
	if c.flights[key] == f {
		delete(c.flights, key)
	}

	if errors.Is(err, ErrNotModified) {
		c.count(key, obs.Revalidated)
		if !f.hadPrev {
			f.err = fmt.Errorf("fetch %q: %w, but nothing was cached", key, err)
			return
		}
		e, err = f.prev, nil
		e.FetchedAt, e.Fallback = time.Now(), nil
	}
	if err != nil {
		f.err = err
		return
	}
	if e.FetchedAt.IsZero() {
		e.FetchedAt = time.Now()
	}
	f.entry = e
	if f.abandoned {
		return
	}
	switch cur, ok := c.items[key]; {
	case !ok || cur.version == f.version:
		c.set(key, e)
	case cur.written == f.written:
		// The entry was only invalidated since. The result is newer than
		// what the cache holds, so it replaces that, but it may predate
		// what the invalidation was for, so it stays stale and the next
		// Fetch asks again, with its validators.
		c.set(key, e)
		c.markStale(c.items[key])
	}
}

// leave removes one waiter from f and cancels f when nobody is left.
func (c *Cache[V]) leave(key string, f *flight[V]) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f.waiters--
	if f.waiters > 0 {
		return
	}
	f.abandoned = true
	f.cancel()
	// A later Fetch must not join a call that is being canceled.
	if c.flights[key] == f {
		delete(c.flights, key)
	}
}

// logFetch logs at debug level what a Fetch of key found, such as hit or
// stale.
func logFetch(ctx context.Context, key, found string) {
	if obs.Enabled(ctx, slog.LevelDebug) {
		slog.DebugContext(ctx, "cache", "span", "cache.memory", "kind", kindOf(key), "key", key, "found", found)
	}
}

// kindOf returns the kind of what key names, for counting: what comes
// before its first colon, slash or question mark, such as pulls in
// pulls:cli/cli?state=open.
func kindOf(key string) string {
	if i := strings.IndexAny(key, ":/?"); i > 0 {
		return key[:i]
	}
	return "other"
}
