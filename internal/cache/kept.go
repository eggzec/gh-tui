package cache

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"
	"time"
)

// Catalog is a Store that can also go through its objects, and read and
// rewrite them without counting as a use, such as the disk layer in
// package disk. A Shelf in a Catalog can list what it keeps, so that its
// entries are revalidated in the background, and doing so doesn't keep
// them from being evicted.
type Catalog interface {
	Store
	// List yields the name of every object of kind with when it was last
	// used.
	List(kind string) iter.Seq2[string, time.Time]
	// Peek is Get, and Replace is Put, without counting as a use.
	Peek(kind, key string) ([]byte, bool)
	Replace(kind, key string, data []byte) error
}

// Kept is what a Shelf says about an entry it keeps.
type Kept struct {
	// Key, Source, ETag, LastModified and FetchedAt are those of the
	// entry.
	Key          string
	Source       string
	ETag         string
	LastModified string
	FetchedAt    time.Time
	// UsedAt is when the entry was last saved or loaded, as far as the
	// store tells: a store may note a use only once in a while.
	UsedAt time.Time
}

// index remembers what a Shelf read of each object it keeps, by name, so
// that listing them again reads only those that changed.
type index struct {
	mu      sync.Mutex
	objects map[string]indexed
}

type indexed struct {
	usedAt time.Time
	kept   Kept
	// ok is false for an object that isn't one of the shelf's entries.
	ok bool
}

func (x *index) get(name string, usedAt time.Time) (indexed, bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	o, ok := x.objects[name]
	return o, ok && o.usedAt.Equal(usedAt)
}

func (x *index) set(name string, o indexed) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.objects == nil {
		x.objects = make(map[string]indexed)
	}
	x.objects[name] = o
}

// update records what the shelf just wrote of the object in name, which a
// Replace wrote without changing when it was used.
func (x *index) update(name string, e Kept) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if o, ok := x.objects[name]; ok && o.ok {
		e.UsedAt = o.kept.UsedAt
		o.kept = e
		x.objects[name] = o
	}
}

// retain forgets the objects not in names.
func (x *index) retain(names map[string]bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	for name := range x.objects {
		if !names[name] {
			delete(x.objects, name)
		}
	}
}

// Kept yields what the shelf keeps, without the values, if its store is a
// Catalog; otherwise it yields nothing. It decodes an entry only the first
// time, and again once it was used or saved since, so it is cheap to call
// again, but it reads the store, so call it where I/O is fine. Reading
// entries this way doesn't count as using them.
func (s *Shelf[V]) Kept() iter.Seq[Kept] {
	return func(yield func(Kept) bool) {
		if s == nil {
			return
		}
		cat, ok := s.store.(Catalog)
		if !ok {
			return
		}
		seen := make(map[string]bool)
		complete := true
		defer func() {
			if complete {
				s.index.retain(seen)
			}
		}()
		for name, usedAt := range cat.List(s.kind) {
			seen[name] = true
			o, ok := s.index.get(name, usedAt)
			if !ok {
				o = s.read(cat, name, usedAt)
				s.index.set(name, o)
			}
			if o.ok && !yield(o.kept) {
				complete = false
				return
			}
		}
	}
}

// read decodes what the object in name says about its entry.
func (s *Shelf[V]) read(cat Catalog, name string, usedAt time.Time) indexed {
	data, ok := cat.Peek(s.kind, name)
	if !ok {
		return indexed{usedAt: usedAt}
	}
	m, err := ReadMeta(data)
	if err != nil || m.Schema != s.schema || objectName(m.Key) != name {
		return indexed{usedAt: usedAt}
	}
	return indexed{usedAt: usedAt, ok: true, kept: Kept{
		Key:          m.Key,
		Source:       m.Source,
		ETag:         m.ETag,
		LastModified: m.LastModified,
		FetchedAt:    m.FetchedAt,
		UsedAt:       usedAt,
	}}
}

// Recheck is what Shelf.Recheck found.
type Recheck int

// What Recheck finds.
const (
	// RecheckSkipped means nothing was asked: c holds the entry fresh, or
	// a fetch of it was running already, or it has no validators, or
	// nothing is kept.
	RecheckSkipped Recheck = iota
	// RecheckNotModified means fn reported ErrNotModified.
	RecheckNotModified
	// RecheckChanged means fn brought a new entry.
	RecheckChanged
)

// Recheck asks fn whether the entry under key is still current, with its
// validators, as Fetch does. If c holds the entry, stale, it is revalidated
// in c, sharing a fetch that is running already; otherwise the kept entry
// is, and c is left alone. Either way the shelf then keeps the entry as
// fetched now: the one GitHub confirmed, if the shelf kept the same, or the
// new one. That doesn't count as using it.
//
// Recheck returns the entry, and what it found. fn must not keep the entry
// on the shelf itself, and its errors are returned as they are.
func (s *Shelf[V]) Recheck(ctx context.Context, c *Cache[V], key string, fn FetchFunc[V]) (Entry[V], Recheck, error) {
	if s == nil {
		return Entry[V]{}, RecheckSkipped, nil
	}
	switch e, st := c.peek(key); st {
	case Fresh:
		return e, RecheckSkipped, nil
	case Stale:
		if !validated(e) {
			return e, RecheckSkipped, nil
		}
		return s.recheckCached(ctx, c, key, fn)
	case Miss:
	}

	kept, ok := s.load(key, s.peek)
	if !ok || !validated(kept) {
		return kept, RecheckSkipped, nil
	}
	e, err := fn(ctx, kept, true)
	switch {
	case errors.Is(err, ErrNotModified):
		kept.FetchedAt = time.Now()
		s.replace(key, kept)
		return kept, RecheckNotModified, nil
	case err != nil:
		return Entry[V]{}, RecheckSkipped, err
	}
	if e.FetchedAt.IsZero() {
		e.FetchedAt = time.Now()
	}
	s.replace(key, e)
	return e, RecheckChanged, nil
}

// recheckCached is Recheck of the entry that c holds under key.
func (s *Shelf[V]) recheckCached(ctx context.Context, c *Cache[V], key string, fn FetchFunc[V]) (Entry[V], Recheck, error) {
	var (
		found atomic.Int32
		prev  Entry[V]
	)
	e, err := c.Fetch(ctx, key, func(ctx context.Context, p Entry[V], ok bool) (Entry[V], error) {
		prev = p
		e, err := fn(ctx, p, ok)
		switch {
		case errors.Is(err, ErrNotModified):
			found.Store(int32(RecheckNotModified))
		case err == nil:
			found.Store(int32(RecheckChanged))
		}
		return e, err
	})
	if err != nil {
		return Entry[V]{}, RecheckSkipped, err
	}
	switch r := Recheck(found.Load()); r {
	case RecheckNotModified:
		// What c holds may carry a change GitHub hasn't confirmed, so the
		// shelf keeps what it kept, if GitHub confirmed that.
		if kept, ok := s.load(key, s.peek); ok && kept.ETag == prev.ETag && kept.LastModified == prev.LastModified {
			kept.FetchedAt = e.FetchedAt
			s.replace(key, kept)
		}
		return e, r, nil
	case RecheckChanged:
		s.replace(key, e)
		return e, r, nil
	default:
		return e, RecheckSkipped, nil
	}
}

// validated reports whether e has validators to ask with.
func validated[V any](e Entry[V]) bool {
	return e.ETag != "" || e.LastModified != ""
}

// peek reads the store without counting as a use, if it can.
func (s *Shelf[V]) peek(kind, name string) ([]byte, bool) {
	if cat, ok := s.store.(Catalog); ok {
		return cat.Peek(kind, name)
	}
	return s.store.Get(kind, name)
}

// replace keeps e under key without counting as a use, if the store can.
// The shelf is only a shortcut, so a failure is ignored.
func (s *Shelf[V]) replace(key string, e Entry[V]) {
	put := s.store.Put
	if cat, ok := s.store.(Catalog); ok {
		put = cat.Replace
	}
	if s.save(key, e, put) == nil {
		s.index.update(objectName(key), Kept{
			Key: key, Source: e.Source, ETag: e.ETag, LastModified: e.LastModified, FetchedAt: e.FetchedAt,
		})
	}
}
