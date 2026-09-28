package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
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
	index  index
	// logs throttles the records of entries dropped, of which a schema
	// bump drops every one.
	logs *obs.Throttle
}

// shelfLogEvery is how often a Shelf logs entries dropped for each
// reason.
const shelfLogEvery = 10 * time.Minute

// NewShelf returns a shelf that keeps entries of kind in store, or nil if
// store is nil. Bump schema whenever V changes in a way that makes older
// entries decode wrongly, and they read as misses.
func NewShelf[V any](store Store, kind string, schema int) *Shelf[V] {
	if store == nil {
		return nil
	}
	return &Shelf[V]{store: store, kind: kind, schema: schema, logs: obs.NewThrottle(shelfLogEvery)}
}

// Load returns the entry kept under key. An entry that can't be decoded, or
// was kept with another format or schema, is removed and reads as a miss.
func (s *Shelf[V]) Load(key string) (Entry[V], bool) {
	if s == nil {
		return Entry[V]{}, false
	}
	return s.load(key, s.store.Get)
}

// load is Load with get reading the store.
func (s *Shelf[V]) load(key string, get func(kind, key string) ([]byte, bool)) (Entry[V], bool) {
	name := objectName(key)
	data, ok := get(s.kind, name)
	if !ok {
		return Entry[V]{}, false
	}
	var r record[V]
	if why := s.mismatch(json.Unmarshal(data, &r), r.Meta, key); why != "" {
		s.store.Delete(s.kind, name)
		s.dropped(context.Background(), why, "")
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

// mismatch returns why an entry that decoded with err, which says m of
// itself, can't be read as the entry of key, or "" if it can.
func (s *Shelf[V]) mismatch(err error, m Meta, key string) string {
	switch {
	case err != nil:
		return "decode"
	case m.Format != format:
		return "format"
	case m.Schema != s.schema:
		return "schema"
	case m.Key != key:
		return "key"
	}
	return ""
}

// Drop forgets the entry kept under key, as Delete does, since GitHub
// refused it for why, such as "not found", and logs it if there was one:
// a list that no longer comes back from disk is then explained.
func (s *Shelf[V]) Drop(ctx context.Context, key, why string) {
	if s == nil {
		return
	}
	name := objectName(key)
	if _, ok := s.peek(s.kind, name); !ok {
		return
	}
	s.store.Delete(s.kind, name)
	s.dropped(ctx, why, key)
}

// dropped counts an entry dropped for why, and logs it once every
// shelfLogEvery for each reason: at info level, since it happens after an
// upgrade or a refusal, and at warn level for one that couldn't be read,
// or was kept under another key. key is the entry's, if the caller knows
// it.
func (s *Shelf[V]) dropped(ctx context.Context, why, key string) {
	obs.CountCache(s.kind, obs.DiskDropped)
	level := slog.LevelInfo
	if why == "decode" || why == "key" {
		level = slog.LevelWarn
	}
	if !obs.Enabled(ctx, level) {
		return
	}
	ok, held := s.logs.Allow(why)
	if !ok {
		return
	}
	attrs := []any{"span", "cache.disk", "kind", s.kind, "reason", why}
	if key != "" {
		attrs = append(attrs, "entry", obs.LogKey(key))
	}
	slog.Log(ctx, level, "kept dropped", append(attrs, obs.Suppressed(held)...)...)
}

// Warm puts the entry kept under key into c, unless c has an entry for key
// already, as Seed does: fresh if it was fetched or revalidated within the
// TTL of c, and stale otherwise. It returns the entry, stale, so that the
// caller may serve it at once, and reports false for a fresh one, which is
// in c, where Fetch finds it.
//
// Until something writes over the stale entry, such as a fetch, Warm
// returns it to every caller, so that readers that start at once all serve
// it at once, however they race to read the store. Each then reads again
// with again set, for which Warm only puts the entry in c and reports
// false, and those reads share one Fetch that revalidates it; so does a
// caller that needs the entry in c rather than served. It reads the store
// only when c misses, so call it where I/O is fine, such as in a tea.Cmd,
// rather than in a Cached read.
func (s *Shelf[V]) Warm(c *Cache[V], key string, again bool) (Entry[V], bool) {
	if s == nil {
		return Entry[V]{}, false
	}
	if _, st := c.Get(key); st == Miss {
		if e, ok := s.Load(key); ok && c.Seed(key, e) {
			if _, st := c.Get(key); st == Stale && obs.Enabled(context.Background(), slog.LevelDebug) {
				slog.Debug("cache", "span", "cache.disk", "kind", kindOf(key), "key", obs.LogKey(key), "found", "stale", "fetched_at", e.FetchedAt)
			}
		}
	}
	if again {
		return Entry[V]{}, false
	}
	e, first, ok := c.seeded(key)
	if !ok {
		return Entry[V]{}, false
	}
	if first {
		obs.CountCache(kindOf(key), obs.StaleServed)
	}
	return e, true
}

// Save keeps e under key, replacing what was kept. A zero FetchedAt is
// saved as the current time.
func (s *Shelf[V]) Save(key string, e Entry[V]) error {
	if s == nil {
		return nil
	}
	return s.save(key, e, s.store.Put)
}

// save is Save with put writing the store.
func (s *Shelf[V]) save(key string, e Entry[V], put func(kind, key string, data []byte) error) error {
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
		// The store never sees it, so it is told here.
		obs.CountCache(s.kind, obs.DiskWriteFailed)
		if ok, held := s.logs.Allow("encode"); ok {
			slog.Warn("disk cache write failed", append([]any{"span", "cache.disk", "kind", s.kind, "op", "encode",
				"err", err.Error()}, obs.Suppressed(held)...)...)
		}
		return fmt.Errorf("keep %s %q: %w", s.kind, key, err)
	}
	if err := put(s.kind, objectName(key), data); err != nil {
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
	if err := json.Unmarshal(withoutValue(data), &m); err != nil {
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

// withoutValue returns the object in data without its value, which Save
// writes last, so that reading what an entry says about itself doesn't
// decode what may be a long list. It returns data if it can't tell.
func withoutValue(data []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return data
	}
	for dec.More() {
		off := dec.InputOffset()
		t, err := dec.Token()
		if err != nil {
			return data
		}
		if t == "value" {
			return slices.Concat(bytes.TrimRight(data[:off], " \t\r\n,"), []byte("}"))
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return data
		}
	}
	return data
}
