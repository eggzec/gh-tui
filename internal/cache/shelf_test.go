package cache

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"
)

// memStore is a Store in memory.
type memStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMemStore() *memStore {
	return &memStore{objects: make(map[string][]byte)}
}

func (m *memStore) Get(kind, key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objects[kind+"/"+key]
	return bytes.Clone(b), ok
}

func (m *memStore) Put(kind, key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[kind+"/"+key] = bytes.Clone(data)
	return nil
}

func (m *memStore) Delete(kind, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, kind+"/"+key)
}

func (m *memStore) names() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Sorted(maps.Keys(m.objects))
}

// only returns the one object in the store.
func (m *memStore) only(t *testing.T) (name string, data []byte) {
	t.Helper()
	names := m.names()
	if len(names) != 1 {
		t.Fatalf("store holds %q, want one object", names)
	}
	d, _ := m.Get(names[0][:len("page")], names[0][len("page/"):])
	return names[0], d
}

type page struct {
	Items []string
	Next  string
}

func TestShelfRoundTrip(t *testing.T) {
	store := newMemStore()
	s := NewShelf[page](store, "page", 1)
	at := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	want := Entry[page]{
		Value:        page{Items: []string{"a", "b"}, Next: "c"},
		ETag:         `"abc"`,
		LastModified: "Thu, 24 Sep 2026 10:00:00 GMT",
		Source:       "https://api.github.com/repos/o/r/issues?per_page=30",
		FetchedAt:    at,
		Tags:         []string{"repo:o/r", "issue:o/r#1"},
	}
	if err := s.Save("list:o/r", want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok := NewShelf[page](store, "page", 1).Load("list:o/r")
	if !ok {
		t.Fatal("Load = miss, want the saved entry")
	}
	if !got.FetchedAt.Equal(at) {
		t.Errorf("FetchedAt = %v, want %v", got.FetchedAt, at)
	}
	got.FetchedAt = at
	if !slices.Equal(got.Value.Items, want.Value.Items) || got.Value.Next != want.Value.Next ||
		got.ETag != want.ETag || got.LastModified != want.LastModified || got.Source != want.Source ||
		!slices.Equal(got.Tags, want.Tags) {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
	if _, ok := s.Load("list:o/other"); ok {
		t.Error("Load of another key = hit, want miss")
	}
}

func TestShelfNamesByHash(t *testing.T) {
	store := newMemStore()
	s := NewShelf[page](store, "page", 1)
	if err := s.Save("list:o/r?cursor=https://api.github.com/x/../y", Entry[page]{}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	name, _ := store.only(t)
	if want := "page/" + objectName("list:o/r?cursor=https://api.github.com/x/../y"); name != want || len(name) != len("page/")+64 {
		t.Errorf("object = %q, want %q", name, want)
	}
}

func TestShelfFillsFetchedAt(t *testing.T) {
	store := newMemStore()
	s := NewShelf[page](store, "page", 1)
	before := time.Now()
	_ = s.Save("k", Entry[page]{})
	e, _ := s.Load("k")
	if e.FetchedAt.Before(before) {
		t.Errorf("FetchedAt = %v, want the time of Save", e.FetchedAt)
	}
}

func TestShelfUnreadable(t *testing.T) {
	tests := []struct {
		name string
		// damage changes the saved object.
		damage func(data []byte) []byte
		// shelf reads it back.
		shelf func(Store) *Shelf[page]
	}{
		{"corrupt", func(d []byte) []byte { return d[:len(d)/2] }, nil},
		{"not json", func([]byte) []byte { return []byte("\x1f\x8b garbage") }, nil},
		{"other format", func(d []byte) []byte {
			return bytes.Replace(d, []byte(`"format":1`), []byte(`"format":0`), 1)
		}, nil},
		{"other key", func(d []byte) []byte {
			return bytes.Replace(d, []byte(`"key":"k"`), []byte(`"key":"j"`), 1)
		}, nil},
		{"other schema", nil, func(st Store) *Shelf[page] { return NewShelf[page](st, "page", 2) }},
		{"other shape", func(d []byte) []byte {
			return bytes.Replace(d, []byte(`"Items":["a"]`), []byte(`"Items":{"a":1}`), 1)
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemStore()
			s := NewShelf[page](store, "page", 1)
			if err := s.Save("k", Entry[page]{Value: page{Items: []string{"a"}}}); err != nil {
				t.Fatal(err)
			}
			if tt.damage != nil {
				_, data := store.only(t)
				_ = store.Put("page", objectName("k"), tt.damage(data))
			}
			if tt.shelf != nil {
				s = tt.shelf(store)
			}
			if e, ok := s.Load("k"); ok {
				t.Errorf("Load = %+v, want a miss", e)
			}
			if names := store.names(); len(names) != 0 {
				t.Errorf("store holds %q after the miss, want it removed", names)
			}
		})
	}
}

func TestShelfNil(t *testing.T) {
	s := NewShelf[page](nil, "page", 1)
	if s != nil {
		t.Fatal("NewShelf(nil) != nil")
	}
	if err := s.Save("k", Entry[page]{}); err != nil {
		t.Errorf("Save = %v, want nil", err)
	}
	if _, ok := s.Load("k"); ok {
		t.Error("Load = hit, want miss")
	}
	s.Delete("k")
}

func TestShelfDelete(t *testing.T) {
	store := newMemStore()
	s := NewShelf[page](store, "page", 1)
	_ = s.Save("k", Entry[page]{})
	s.Delete("k")
	if _, ok := s.Load("k"); ok {
		t.Error("Load after Delete = hit, want miss")
	}
}

func TestReadMeta(t *testing.T) {
	store := newMemStore()
	s := NewShelf[page](store, "page", 3)
	_ = s.Save("k", Entry[page]{Value: page{Items: []string{"a"}}, ETag: `"e"`, Source: "https://api.github.com/x", Tags: []string{"t"}})
	_, data := store.only(t)
	m, err := ReadMeta(data)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if m.Key != "k" || m.Schema != 3 || m.ETag != `"e"` || m.Source != "https://api.github.com/x" || !slices.Equal(m.Tags, []string{"t"}) || m.FetchedAt.IsZero() {
		t.Errorf("ReadMeta = %+v", m)
	}
	if _, err := ReadMeta([]byte(`{"format":99}`)); err == nil {
		t.Error("ReadMeta of another format succeeded, want an error")
	}
	if _, err := ReadMeta([]byte("nope")); err == nil {
		t.Error("ReadMeta of garbage succeeded, want an error")
	}
	// The value is skipped, however it looks.
	m, err = ReadMeta([]byte(`{"format":1, "schema":2, "key":"k", "value":{"value":[1,"}"]}, "etag":"late"}`))
	if err != nil || m.Key != "k" || m.Schema != 2 {
		t.Errorf("ReadMeta of a nested value = %+v, %v", m, err)
	}
	if _, err := ReadMeta([]byte(`{"format":1,"value":`)); err != nil {
		t.Errorf("ReadMeta of a truncated value = %v, want the meta before it", err)
	}
}

func TestSeed(t *testing.T) {
	c := New[string]()
	at := time.Now().Add(-DefaultTTL)
	if !c.Seed("k", Entry[string]{Value: "kept", ETag: `"e"`, FetchedAt: at, Tags: []string{"t"}}) {
		t.Fatal("Seed into an empty cache = false, want true")
	}
	e, st := c.Get("k")
	if st != Stale || e.Value != "kept" || !e.FetchedAt.Equal(at) {
		t.Errorf("Get = %+v, %v; want the kept entry, stale", e, st)
	}
	if got := c.Tagged("t"); !slices.Equal(got, []string{"kept"}) {
		t.Errorf("Tagged = %q, want the seeded entry", got)
	}
	if c.Seed("k", Entry[string]{Value: "other"}) {
		t.Error("Seed over an entry = true, want false")
	}
	if e, _ := c.Get("k"); e.Value != "kept" {
		t.Errorf("value = %q after a second Seed, want it kept", e.Value)
	}
}

func TestSeedState(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		at   time.Time
		want State
	}{
		{"within the TTL", now.Add(-time.Second), Fresh},
		{"past the TTL", now.Add(-DefaultTTL), Stale},
		{"no fetch time", time.Time{}, Stale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New[string]()
			c.Seed("k", Entry[string]{Value: "kept", FetchedAt: tt.at})
			if _, st := c.Get("k"); st != tt.want {
				t.Errorf("state = %v, want %v", st, tt.want)
			}
		})
	}
}

func TestSeedThenFetchRevalidates(t *testing.T) {
	c := New[string]()
	c.Seed("k", Entry[string]{Value: "kept", ETag: `"e"`})
	e, err := c.Fetch(t.Context(), "k", func(_ context.Context, prev Entry[string], ok bool) (Entry[string], error) {
		if !ok || prev.ETag != `"e"` {
			t.Errorf("prev = %+v, %v; want the seeded entry", prev, ok)
		}
		return Entry[string]{}, ErrNotModified
	})
	if err != nil || e.Value != "kept" {
		t.Errorf("Fetch = %+v, %v; want the seeded value", e, err)
	}
	if _, st := c.Get("k"); st != Fresh {
		t.Errorf("state after a 304 = %v, want fresh", st)
	}
}

func TestSeedDuringFetch(t *testing.T) {
	c := New[string]()
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.Fetch(context.Background(), "k", func(context.Context, Entry[string], bool) (Entry[string], error) {
			close(started)
			<-release
			return Entry[string]{Value: "new"}, nil
		})
	}()
	<-started
	if c.Seed("k", Entry[string]{Value: "kept"}) {
		t.Error("Seed during a fetch = true, want false")
	}
	close(release)
	<-done
	if e, st := c.Get("k"); e.Value != "new" || st != Fresh {
		t.Errorf("Get = %+v, %v; want the fetched value", e, st)
	}
}

func TestShelfWarm(t *testing.T) {
	store := newMemStore()
	s := NewShelf[page](store, "page", 1)
	_ = s.Save("k", Entry[page]{Value: page{Next: "kept"}, ETag: `"e"`, FetchedAt: time.Now().Add(-DefaultTTL), Tags: []string{"t"}})
	c := New[page]()

	e, ok := s.Warm(c, "k")
	if !ok || e.Value.Next != "kept" {
		t.Fatalf("Warm = %+v, %v; want the kept entry", e, ok)
	}
	if got, st := c.Get("k"); st != Stale || got.ETag != `"e"` {
		t.Errorf("Get after Warm = %+v, %v; want the kept entry, stale", got, st)
	}
	if _, ok := s.Warm(c, "k"); ok {
		t.Error("second Warm = true, want false: memory has the entry")
	}
	if _, ok := s.Warm(c, "missing"); ok {
		t.Error("Warm of a key never kept = true, want false")
	}
	var none *Shelf[page]
	if _, ok := none.Warm(c, "k"); ok {
		t.Error("Warm of a nil shelf = true, want false")
	}
}

func TestShelfWarmFresh(t *testing.T) {
	s := NewShelf[page](newMemStore(), "page", 1)
	_ = s.Save("k", Entry[page]{Value: page{Next: "kept"}, ETag: `"e"`})
	c := New[page]()
	if _, ok := s.Warm(c, "k"); ok {
		t.Error("Warm of an entry kept within the TTL = true, want false: it needs no revalidation")
	}
	if e, st := c.Get("k"); st != Fresh || e.Value.Next != "kept" {
		t.Errorf("Get after Warm = %+v, %v; want the kept entry, fresh", e, st)
	}
}
