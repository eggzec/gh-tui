// Package cache provides an in-memory LRU cache with TTL freshness and HTTP
// validator metadata, for stale-while-revalidate reads, and a Shelf that
// keeps entries in a Store across sessions.
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
	// Source is the URL the value was read from, if a plain GET of it with
	// the validators revalidates the entry. A Shelf keeps it, so that the
	// entry can be revalidated without knowing what it holds.
	Source    string
	FetchedAt time.Time
	Tags      []string
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
	// size measures an entry for opts.maxSize, or is nil; total is the size
	// of every entry.
	size  func(V) int64
	total int64
	// seq numbers writes, so code that read an entry earlier can tell
	// whether it changed since.
	seq     uint64
	flights map[string]*flight[V]
	// root is the sentinel of a circular list ordered from most recently
	// used (root.next) to least recently used (root.prev).
	root node[V]
}

type node[V any] struct {
	key        string
	entry      Entry[V]
	size       int64
	stale      bool
	version    uint64
	prev, next *node[V]
}

// New returns an empty cache.
func New[V any](opts ...Option) *Cache[V] {
	o := options{capacity: DefaultCapacity, ttl: DefaultTTL}
	for _, opt := range opts {
		opt(&o)
	}
	c := &Cache[V]{opts: o, items: make(map[string]*node[V])}
	c.size, _ = o.size.(func(V) int64)
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

// Seed stores e under key, unless key holds an entry already or is being
// fetched, and reports whether it did. It is for entries kept from an
// earlier session, such as by a Shelf. An entry fetched, or revalidated,
// within the TTL is fresh, as if this session had fetched it; an older one,
// or one without a FetchedAt, is stale: it is shown at once, and the next
// Fetch revalidates it with its validators.
func (c *Cache[V]) Seed(key string, e Entry[V]) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; ok {
		return false
	}
	// The fetch would not store its result over an entry it didn't start
	// from, and it brings a newer one.
	if _, ok := c.flights[key]; ok {
		return false
	}
	stale := e.FetchedAt.IsZero()
	c.set(key, e)
	if stale {
		c.markStale(c.items[key])
	}
	return true
}

// Invalidate marks the entry for key as stale.
func (c *Cache[V]) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n, ok := c.items[key]; ok {
		c.markStale(n)
	}
}

// Tagged returns the values of every entry tagged with tag, fresh or stale,
// in no particular order. It is a snapshot, so the caller may use the cache
// while going through it.
func (c *Cache[V]) Tagged(tag string) []V {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []V
	for _, n := range c.items {
		if slices.Contains(n.entry.Tags, tag) {
			out = append(out, n.entry.Value)
		}
	}
	return out
}

// TaggedEntries returns the entries tagged with tag by key, fresh or stale.
// Like Tagged, it is a snapshot.
func (c *Cache[V]) TaggedEntries(tag string) map[string]Entry[V] {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]Entry[V])
	for key, n := range c.items {
		if slices.Contains(n.entry.Tags, tag) {
			out[key] = n.entry
		}
	}
	return out
}

// InvalidateTag marks every entry tagged with tag as stale. Entries are kept
// rather than removed so views can keep showing them, and revalidate them
// cheaply with their validators.
func (c *Cache[V]) InvalidateTag(tag string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, n := range c.items {
		if slices.Contains(n.entry.Tags, tag) {
			c.markStale(n)
		}
	}
}

// Size returns the total size of the entries, as measured by the size
// function of WithMaxSize, or 0 without one.
func (c *Cache[V]) Size() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
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
	c.seq++
	if n, ok := c.items[key]; ok {
		n.entry, n.stale, n.version = e, false, c.seq
		c.resize(n)
		c.moveToFront(n)
		c.shrink()
		return
	}
	var n *node[V]
	if len(c.items) < c.opts.capacity {
		n = new(node[V])
	} else {
		// Reuse the evicted node so a full cache doesn't allocate on Set.
		n = c.root.prev
		c.remove(n)
	}
	*n = node[V]{key: key, entry: e, version: c.seq}
	c.items[key] = n
	c.pushFront(n)
	c.resize(n)
	c.shrink()
}

// resize measures the value of n again. c.mu must be held.
func (c *Cache[V]) resize(n *node[V]) {
	if c.size == nil {
		return
	}
	s := c.size(n.entry.Value)
	c.total += s - n.size
	n.size = s
}

// shrink evicts the least recently used entries while they are larger than
// the size limit, down to the most recent one. c.mu must be held.
func (c *Cache[V]) shrink() {
	for c.size != nil && c.total > c.opts.maxSize && len(c.items) > 1 {
		c.remove(c.root.prev)
	}
}

// remove unlinks n and forgets it. c.mu must be held.
func (c *Cache[V]) remove(n *node[V]) {
	c.unlink(n)
	delete(c.items, n.key)
	c.total -= n.size
}

// markStale counts as a write, so a Fetch that started earlier doesn't make
// the entry fresh again with data from before the invalidation.
func (c *Cache[V]) markStale(n *node[V]) {
	c.seq++
	n.stale, n.version = true, c.seq
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
