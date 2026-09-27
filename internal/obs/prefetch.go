package obs

import (
	"context"
	"sync"
)

// maxPrefetched bounds what a Prefetched remembers. Past it, it forgets
// all and starts over, which only undercounts what gets opened.
const maxPrefetched = 4096

// Prefetched remembers what was read ahead of its use, such as the details
// of the first rows of a list, so that the summary can tell how much of it
// was then opened. A nil *Prefetched remembers nothing. It is safe for
// concurrent use.
type Prefetched[K comparable] struct {
	kind string

	mu   sync.Mutex
	keys map[K]prefetchState
}

// prefetchState is what a Prefetched knows of a key.
type prefetchState struct {
	// pending counts the reads of the key started and not yet ended.
	pending int
	// read is set once a read of the key brought it, and opened once the
	// key was opened while it was being read.
	read, opened bool
}

// NewPrefetched returns a Prefetched that counts under kind, such as
// "pull" or "file".
func NewPrefetched[K comparable](kind string) *Prefetched[K] {
	return &Prefetched[K]{kind: kind, keys: make(map[K]prefetchState)}
}

// Kind returns the kind p counts under.
func (p *Prefetched[K]) Kind() string {
	if p == nil {
		return ""
	}
	return p.kind
}

// Count counts e under p's kind.
func (p *Prefetched[K]) Count(e PrefetchEvent) {
	if p != nil {
		CountPrefetch(p.kind, e)
	}
}

// Started records that a read of k ahead started, so that opening k before
// it ends counts as a use once the read brings it. End the read with Read
// or Dropped.
func (p *Prefetched[K]) Started(k K) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.keys) >= maxPrefetched {
		clear(p.keys)
	}
	st := p.keys[k]
	st.pending++
	p.keys[k] = st
}

// Read records that k was read ahead and brought what it was after. If k
// was opened while it was read, that counts as its use now.
func (p *Prefetched[K]) Read(k K) {
	if p == nil {
		return
	}
	CountPrefetch(p.kind, PrefetchRead)
	p.mu.Lock()
	if len(p.keys) >= maxPrefetched {
		clear(p.keys)
	}
	st := p.keys[k]
	st.pending = max(st.pending-1, 0)
	opened := st.opened
	if opened {
		delete(p.keys, k)
	} else {
		st.read = true
		p.keys[k] = st
	}
	p.mu.Unlock()
	if opened {
		CountPrefetch(p.kind, PrefetchOpened)
	}
}

// Dropped records that a read of k ahead ended without bringing it, such
// as one canceled or failed.
func (p *Prefetched[K]) Dropped(k K) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.keys[k]
	if !ok {
		return
	}
	st.pending = max(st.pending-1, 0)
	switch {
	case st.pending == 0 && !st.read:
		delete(p.keys, k)
	default:
		p.keys[k] = st
	}
}

// Opened records that k was opened, which counts once for what was read
// ahead: at once if it was read, or once its read brings it if one is in
// flight.
func (p *Prefetched[K]) Opened(k K) {
	if p == nil {
		return
	}
	p.mu.Lock()
	st, ok := p.keys[k]
	switch {
	case !ok:
	case st.read:
		delete(p.keys, k)
	case st.pending > 0:
		st.opened = true
		p.keys[k] = st
	}
	p.mu.Unlock()
	if ok && st.read {
		CountPrefetch(p.kind, PrefetchOpened)
	}
}

type prefetchKey struct{}

// ForPrefetch returns ctx marked as a read ahead of its use, which the
// client lets use fewer of its requests in flight than what the user waits
// for, and whose GraphQL points count against the prefetch budget.
func ForPrefetch(ctx context.Context) context.Context {
	return context.WithValue(ctx, prefetchKey{}, true)
}

// IsPrefetch reports whether ctx is marked as a read ahead.
func IsPrefetch(ctx context.Context) bool {
	v, _ := ctx.Value(prefetchKey{}).(bool)
	return v
}

type backgroundKey struct{}

// ForBackground returns ctx marked as the work of a loop that runs on its
// own, such as the revalidator's passes or the sync engine's polls. The
// client doesn't send its requests again when they fail, since the loop
// comes back to them anyway, unlike a read the user waits for, and lets
// them use fewer of its requests in flight.
func ForBackground(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundKey{}, true)
}

// IsBackground reports whether ctx is marked as the work of a background
// loop.
func IsBackground(ctx context.Context) bool {
	v, _ := ctx.Value(backgroundKey{}).(bool)
	return v
}
