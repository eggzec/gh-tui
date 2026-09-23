// Package cache provides an in-memory LRU cache with TTL freshness and HTTP
// validator metadata, for stale-while-revalidate reads.
package cache

import (
	"slices"
	"sync"
	"time"
)

// Entry is a cached value together with the metadata needed to revalidate it.
// The cache does not copy values, so treat entries as immutable.
type Entry[V any] struct {
	Value        V
	ETag         string
	LastModified string
	FetchedAt    time.Time
	Tags         []string
}

// State describes the result of a lookup.
type State int

// Lookup states. A Stale entry is still returned so it can be shown while it
// is revalidated.
const (
	Miss State = iota
	Fresh
	Stale
)

// String returns the state's name.
func (s State) String() string {
	switch s {
	case Miss:
		return "miss"
	case Fresh:
		return "fresh"
	case Stale:
		return "stale"
	default:
		return "unknown"
	}
}

// Cache is a concurrency-safe LRU cache keyed by string. The zero value is not
// usable; create one with New.
type Cache[V any] struct {
	mu    sync.Mutex
	opts  options
	items map[string]*node[V]
	// root is the sentinel of a circular list ordered from most recently
	// used (root.next) to least recently used (root.prev).
	root node[V]
}

type node[V any] struct {
	key        string
	entry      Entry[V]
	stale      bool
	prev, next *node[V]
}

// New returns an empty cache.
func New[V any](opts ...Option) *Cache[V] {
	o := options{capacity: DefaultCapacity, ttl: DefaultTTL}
	for _, opt := range opts {
		opt(&o)
	}
	c := &Cache[V]{opts: o, items: make(map[string]*node[V])}
	c.root.next, c.root.prev = &c.root, &c.root
	return c
}

// Get returns the entry for key and whether it is fresh or stale. A hit marks
// the entry as recently used.
func (c *Cache[V]) Get(key string) (Entry[V], State) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, ok := c.items[key]
	if !ok {
		return Entry[V]{}, Miss
	}
	c.moveToFront(n)
	return n.entry, c.state(n)
}

// Set stores e under key, replacing any previous entry. A zero FetchedAt is
// set to the current time.
func (c *Cache[V]) Set(key string, e Entry[V]) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.set(key, e)
}

// Invalidate marks the entry for key as stale.
func (c *Cache[V]) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n, ok := c.items[key]; ok {
		n.stale = true
	}
}

// InvalidateTag marks every entry tagged with tag as stale. Entries are kept
// rather than removed so views can keep showing them, and revalidate them
// cheaply with their validators.
func (c *Cache[V]) InvalidateTag(tag string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, n := range c.items {
		if slices.Contains(n.entry.Tags, tag) {
			n.stale = true
		}
	}
}

// Len returns the number of entries.
func (c *Cache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// set stores e under key. c.mu must be held.
func (c *Cache[V]) set(key string, e Entry[V]) {
	if e.FetchedAt.IsZero() {
		e.FetchedAt = time.Now()
	}
	if n, ok := c.items[key]; ok {
		n.entry, n.stale = e, false
		c.moveToFront(n)
		return
	}
	var n *node[V]
	if len(c.items) < c.opts.capacity {
		n = new(node[V])
	} else {
		// Reuse the evicted node so a full cache doesn't allocate on Set.
		n = c.root.prev
		c.unlink(n)
		delete(c.items, n.key)
	}
	*n = node[V]{key: key, entry: e}
	c.items[key] = n
	c.pushFront(n)
}

func (c *Cache[V]) state(n *node[V]) State {
	if n.stale || time.Since(n.entry.FetchedAt) >= c.opts.ttl {
		return Stale
	}
	return Fresh
}

func (c *Cache[V]) moveToFront(n *node[V]) {
	if c.root.next == n {
		return
	}
	c.unlink(n)
	c.pushFront(n)
}

func (c *Cache[V]) pushFront(n *node[V]) {
	n.prev, n.next = &c.root, c.root.next
	n.prev.next, n.next.prev = n, n
}

func (c *Cache[V]) unlink(n *node[V]) {
	n.prev.next, n.next.prev = n.next, n.prev
	n.prev, n.next = nil, nil
}
