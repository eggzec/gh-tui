package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Store keeps objects across sessions, such as the disk layer in package
// disk. Kinds are lowercase words and keys are lowercase hex. Get reports
// false for an object it can't read, and Put may fail; a store is only a
// shortcut, so either way the entry is read from the server instead.
type Store interface {
	Get(kind, key string) ([]byte, bool)
	Put(kind, key string, data []byte) error
	Delete(kind, key string)
}

// Meta is what a kept entry says about itself besides its value: enough to
// find out whether it is still current without decoding the value, with
// one conditional request to Source when it has validators.
type Meta struct {
	// Format is the version of the encoding, and Schema that of the value,
	// which the Shelf that saved it was made with.
	Format int `json:"format"`
	Schema int `json:"schema"`
	// Key is the cache key the entry was saved under.
	Key          string    `json:"key"`
	Source       string    `json:"source,omitempty"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	FetchedAt    time.Time `json:"fetched_at"`
	Tags         []string  `json:"tags,omitempty"`
}

// record is how a Shelf encodes an entry.
type record[V any] struct {
	Meta
	Value V `json:"value"`
}

// format is the version of the encoding of every entry. Change it when the
// encoding changes, and older entries read as misses.
const format = 1

var errFormat = errors.New("kept entry of another format")

// Shelf keeps the entries of one kind in a Store, so that a later session
// starts from what an earlier one read. Entries are encoded as JSON and
// named by the SHA-256 of their key, so any key makes a valid name, and a
// Shelf reads only the entries of its own Store: the Store decides who may
// read what, such as one directory per account.
//
// A nil *Shelf keeps nothing, so a service without a Store needs no
// special case. A Shelf is safe for concurrent use.
type Shelf[V any] struct {
	store  Store
	kind   string
	schema int
}

// NewShelf returns a shelf that keeps entries of kind in store, or nil if
// store is nil. Bump schema whenever V changes in a way that makes older
// entries decode wrongly, and they read as misses.
func NewShelf[V any](store Store, kind string, schema int) *Shelf[V] {
	if store == nil {
		return nil
	}
	return &Shelf[V]{store: store, kind: kind, schema: schema}
}

// Load returns the entry kept under key. An entry that can't be decoded, or
// was kept with another format or schema, is removed and reads as a miss.
func (s *Shelf[V]) Load(key string) (Entry[V], bool) {
	if s == nil {
		return Entry[V]{}, false
	}
	name := objectName(key)
	data, ok := s.store.Get(s.kind, name)
	if !ok {
		return Entry[V]{}, false
	}
	var r record[V]
	if err := json.Unmarshal(data, &r); err != nil || r.Format != format || r.Schema != s.schema || r.Key != key {
		s.store.Delete(s.kind, name)
		return Entry[V]{}, false
	}
	return Entry[V]{
		Value:        r.Value,
		ETag:         r.ETag,
		LastModified: r.LastModified,
		Source:       r.Source,
		FetchedAt:    r.FetchedAt,
		Tags:         r.Tags,
	}, true
}

// Warm puts the entry kept under key into c, unless c has an entry for key
// already, as Seed does: fresh if it was fetched or revalidated within the
// TTL of c, and stale otherwise. It returns a stale entry it put, so that a
// caller may serve it at once while the next Fetch revalidates it, and
// reports false otherwise; a fresh one is in c, where Fetch finds it. It
// reads the store only when c misses, so call it where I/O is fine, such as
// in a tea.Cmd, rather than in a Cached read.
func (s *Shelf[V]) Warm(c *Cache[V], key string) (Entry[V], bool) {
	if s == nil {
		return Entry[V]{}, false
	}
	if _, st := c.Get(key); st != Miss {
		return Entry[V]{}, false
	}
	e, ok := s.Load(key)
	if !ok || !c.Seed(key, e) {
		return Entry[V]{}, false
	}
	if _, st := c.Get(key); st != Stale {
		return Entry[V]{}, false
	}
	return e, true
}

// Save keeps e under key, replacing what was kept. A zero FetchedAt is
// saved as the current time.
func (s *Shelf[V]) Save(key string, e Entry[V]) error {
	if s == nil {
		return nil
	}
	if e.FetchedAt.IsZero() {
		e.FetchedAt = time.Now()
	}
	data, err := json.Marshal(record[V]{
		Format:       format,
		Schema:       s.schema,
		Key:          key,
		Source:       e.Source,
		ETag:         e.ETag,
		LastModified: e.LastModified,
		FetchedAt:    e.FetchedAt,
		Tags:         e.Tags,
		Value:        e.Value,
	})
	if err != nil {
		return fmt.Errorf("keep %s %q: %w", s.kind, key, err)
	}
	if err := s.store.Put(s.kind, objectName(key), data); err != nil {
		return fmt.Errorf("keep %s %q: %w", s.kind, key, err)
	}
	return nil
}

// Delete forgets the entry kept under key.
func (s *Shelf[V]) Delete(key string) {
	if s != nil {
		s.store.Delete(s.kind, objectName(key))
	}
}

// ReadMeta decodes what an object that a Shelf saved says about its entry,
// without the value, such as to go through the entries of a Store and
// revalidate those with validators.
func ReadMeta(data []byte) (Meta, error) {
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return Meta{}, fmt.Errorf("read kept entry: %w", err)
	}
	if m.Format != format {
		return Meta{}, errFormat
	}
	return m, nil
}

// objectName names the object of key in the store.
func objectName(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}
