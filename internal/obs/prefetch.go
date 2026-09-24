package obs

import "sync"

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
	keys map[K]struct{}
}

// NewPrefetched returns a Prefetched that counts under kind, such as
// "pull" or "file".
func NewPrefetched[K comparable](kind string) *Prefetched[K] {
	return &Prefetched[K]{kind: kind, keys: make(map[K]struct{})}
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

// Read records that k was read ahead and brought what it was after.
func (p *Prefetched[K]) Read(k K) {
	if p == nil {
		return
	}
	CountPrefetch(p.kind, PrefetchRead)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.keys) >= maxPrefetched {
		clear(p.keys)
	}
	p.keys[k] = struct{}{}
}

// Opened records that k was opened, which counts once for what was read
// ahead.
func (p *Prefetched[K]) Opened(k K) {
	if p == nil {
		return
	}
	p.mu.Lock()
	_, ok := p.keys[k]
	delete(p.keys, k)
	p.mu.Unlock()
	if ok {
		CountPrefetch(p.kind, PrefetchOpened)
	}
}
