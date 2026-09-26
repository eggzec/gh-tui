package cache

import (
	"context"
	"errors"
	"iter"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// memCatalog is a Catalog in memory. Its clock ticks a second with every
// Put, so each one changes when an object was used, and it counts Peeks.
type memCatalog struct {
	*memStore
	mu     sync.Mutex
	clock  time.Time
	used   map[string]time.Time
	peeks  int
	writes []string
}

func newMemCatalog() *memCatalog {
	return &memCatalog{memStore: newMemStore(), clock: time.Unix(1_000_000, 0), used: make(map[string]time.Time)}
}

func (m *memCatalog) Put(kind, key string, data []byte) error {
	m.mu.Lock()
	m.clock = m.clock.Add(time.Second)
	m.used[kind+"/"+key] = m.clock
	m.writes = append(m.writes, "put")
	m.mu.Unlock()
	return m.memStore.Put(kind, key, data)
}

func (m *memCatalog) Replace(kind, key string, data []byte) error {
	m.mu.Lock()
	if _, ok := m.used[kind+"/"+key]; !ok {
		m.clock = m.clock.Add(time.Second)
		m.used[kind+"/"+key] = m.clock
	}
	m.writes = append(m.writes, "replace")
	m.mu.Unlock()
	return m.memStore.Put(kind, key, data)
}

func (m *memCatalog) Peek(kind, key string) ([]byte, bool) {
	m.mu.Lock()
	m.peeks++
	m.mu.Unlock()
	return m.Get(kind, key)
}

func (m *memCatalog) List(kind string) iter.Seq2[string, time.Time] {
	return func(yield func(string, time.Time) bool) {
		for _, name := range m.names() {
			k, key, _ := strings.Cut(name, "/")
			if k != kind {
				continue
			}
			m.mu.Lock()
			at := m.used[name]
			m.mu.Unlock()
			if !yield(key, at) {
				return
			}
		}
	}
}

func (m *memCatalog) counts() (peeks int, writes []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, w := m.peeks, slices.Clone(m.writes)
	m.peeks, m.writes = 0, nil
	return p, w
}

func keptKeys(s *Shelf[page]) []string {
	var keys []string
	for k := range s.Kept() {
		keys = append(keys, k.Key)
	}
	slices.Sort(keys)
	return keys
}

func TestShelfKept(t *testing.T) {
	store := newMemCatalog()
	s := NewShelf[page](store, "page", 2)
	at := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	for i := range 3 {
		_ = s.Save("k"+strconv.Itoa(i), Entry[page]{ETag: `"e` + strconv.Itoa(i) + `"`, Source: "https://api.github.com/x", FetchedAt: at})
	}
	// Entries of another schema or kind, and objects that aren't entries,
	// aren't listed.
	_ = NewShelf[page](store, "page", 1).Save("old", Entry[page]{})
	_ = NewShelf[page](store, "other", 2).Save("other", Entry[page]{})
	_ = store.Put("page", "0123abcd", []byte("garbage"))

	var got []Kept
	for k := range s.Kept() {
		got = append(got, k)
	}
	slices.SortFunc(got, func(a, b Kept) int { return strings.Compare(a.Key, b.Key) })
	if len(got) != 3 {
		t.Fatalf("Kept = %+v, want the 3 entries of the shelf", got)
	}
	k := got[1]
	if k.Key != "k1" || k.ETag != `"e1"` || k.Source != "https://api.github.com/x" || !k.FetchedAt.Equal(at) || k.UsedAt.IsZero() {
		t.Errorf("Kept[1] = %+v", k)
	}

	// A second listing reads only what changed.
	store.counts()
	if keys := keptKeys(s); len(keys) != 3 {
		t.Errorf("second Kept = %q", keys)
	}
	if peeks, _ := store.counts(); peeks != 0 {
		t.Errorf("second Kept read %d objects, want none", peeks)
	}
	_ = s.Save("k1", Entry[page]{ETag: `"new"`})
	_ = s.Save("k3", Entry[page]{ETag: `"e3"`})
	s.Delete("k0")
	if keys := keptKeys(s); !slices.Equal(keys, []string{"k1", "k2", "k3"}) {
		t.Errorf("Kept after changes = %q", keys)
	}
	if peeks, _ := store.counts(); peeks != 2 {
		t.Errorf("Kept after changes read %d objects, want the 2 saved since", peeks)
	}
	for range s.Kept() {
		break // stopping early must not panic
	}
}

func TestShelfKeptWithoutCatalog(t *testing.T) {
	s := NewShelf[page](newMemStore(), "page", 1)
	_ = s.Save("k", Entry[page]{})
	if keys := keptKeys(s); len(keys) != 0 {
		t.Errorf("Kept of a plain store = %q, want nothing", keys)
	}
	var none *Shelf[page]
	if keys := keptKeys(none); len(keys) != 0 {
		t.Errorf("Kept of a nil shelf = %q, want nothing", keys)
	}
}

// answer is a FetchFunc that records its calls and answers with a 304 if
// the validator matches etag, and with value otherwise.
type answer struct {
	etag  string
	value page
	err   error
	calls []string
}

func (a *answer) fetch(_ context.Context, prev Entry[page], ok bool) (Entry[page], error) {
	a.calls = append(a.calls, prev.ETag)
	switch {
	case a.err != nil:
		return Entry[page]{}, a.err
	case ok && prev.ETag == a.etag:
		return Entry[page]{}, ErrNotModified
	}
	return Entry[page]{Value: a.value, ETag: a.etag, Tags: []string{"t"}}, nil
}

func TestRecheck(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	kept := Entry[page]{Value: page{Next: "kept"}, ETag: `"v1"`, FetchedAt: old}
	tests := []struct {
		name string
		// cached is what memory holds, if set, and stale whether it is.
		cached    *Entry[page]
		stale     bool
		noKept    bool
		server    answer
		want      Recheck
		wantValue string
		wantCalls []string
		// wantKept is what the shelf keeps after, and wantWrite how.
		wantKept  string
		wantETag  string
		wantFresh bool
		wantWrite []string
		// wantCached is what memory holds after, if anything.
		wantCached string
	}{
		{
			name: "kept, not modified", server: answer{etag: `"v1"`},
			want: RecheckNotModified, wantValue: "kept", wantCalls: []string{`"v1"`},
			wantKept: "kept", wantETag: `"v1"`, wantFresh: true, wantWrite: []string{"replace"},
		},
		{
			name: "kept, changed", server: answer{etag: `"v2"`, value: page{Next: "new"}},
			want: RecheckChanged, wantValue: "new", wantCalls: []string{`"v1"`},
			wantKept: "new", wantETag: `"v2"`, wantFresh: true, wantWrite: []string{"replace"},
		},
		{
			name: "nothing kept", noKept: true, server: answer{etag: `"v1"`},
			want: RecheckSkipped,
		},
		{
			name: "cached fresh", cached: &Entry[page]{Value: page{Next: "memory"}, ETag: `"v1"`},
			server: answer{etag: `"v1"`}, want: RecheckSkipped, wantValue: "memory",
			wantKept: "kept", wantETag: `"v1"`, wantCached: "memory",
		},
		{
			name: "cached stale, not modified", cached: &Entry[page]{Value: page{Next: "kept"}, ETag: `"v1"`}, stale: true,
			server: answer{etag: `"v1"`}, want: RecheckNotModified, wantValue: "kept", wantCalls: []string{`"v1"`},
			wantKept: "kept", wantETag: `"v1"`, wantFresh: true, wantWrite: []string{"replace"}, wantCached: "kept",
		},
		{
			// Memory holds a change GitHub hasn't confirmed, under
			// validators the shelf doesn't have.
			name: "cached stale with other validators", cached: &Entry[page]{Value: page{Next: "mutated"}, ETag: `"v0"`}, stale: true,
			server: answer{etag: `"v0"`}, want: RecheckNotModified, wantValue: "mutated", wantCalls: []string{`"v0"`},
			wantKept: "kept", wantETag: `"v1"`, wantCached: "mutated",
		},
		{
			name: "cached stale, changed", cached: &Entry[page]{Value: page{Next: "kept"}, ETag: `"v1"`}, stale: true,
			server: answer{etag: `"v2"`, value: page{Next: "new"}}, want: RecheckChanged, wantValue: "new", wantCalls: []string{`"v1"`},
			wantKept: "new", wantETag: `"v2"`, wantFresh: true, wantWrite: []string{"replace"}, wantCached: "new",
		},
		{
			name: "cached without validators", cached: &Entry[page]{Value: page{Next: "memory"}}, stale: true,
			server: answer{etag: `"v1"`}, want: RecheckSkipped, wantValue: "memory",
			wantKept: "kept", wantETag: `"v1"`, wantCached: "memory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemCatalog()
			s := NewShelf[page](store, "page", 1)
			if !tt.noKept {
				_ = s.Save("k", kept)
			}
			c := New[page]()
			if tt.cached != nil {
				c.Set("k", *tt.cached)
				if tt.stale {
					c.Invalidate("k")
				}
			}
			keys := keptKeys(s)
			store.counts()

			e, got, err := s.Recheck(t.Context(), c, "k", tt.server.fetch)
			if err != nil || got != tt.want || e.Value.Next != tt.wantValue {
				t.Errorf("Recheck = %q, %v, %v; want %q, %v", e.Value.Next, got, err, tt.wantValue, tt.want)
			}
			if !slices.Equal(tt.server.calls, tt.wantCalls) {
				t.Errorf("asked with %q, want %q", tt.server.calls, tt.wantCalls)
			}
			if _, writes := store.counts(); !slices.Equal(writes, tt.wantWrite) {
				t.Errorf("writes = %q, want %q", writes, tt.wantWrite)
			}
			if k, ok := s.Load("k"); ok != (tt.wantKept != "") || k.Value.Next != tt.wantKept || k.ETag != tt.wantETag || tt.wantKept != "" && time.Since(k.FetchedAt) < time.Minute != tt.wantFresh {
				t.Errorf("kept = %+v, %v; want %q %s fresh %v", k, ok, tt.wantKept, tt.wantETag, tt.wantFresh)
			}
			if got, st := c.Get("k"); (st != Miss) != (tt.wantCached != "") || got.Value.Next != tt.wantCached {
				t.Errorf("cached = %+v, %v; want %q", got, st, tt.wantCached)
			}
			// The listing learns of the rewrite without reading again.
			if len(keys) == 1 && tt.wantFresh {
				for k := range s.Kept() {
					if time.Since(k.FetchedAt) > time.Minute {
						t.Errorf("Kept after Recheck = %+v, want it fetched now", k)
					}
				}
				if peeks, _ := store.counts(); peeks != 0 {
					t.Errorf("Kept after Recheck read %d objects, want none", peeks)
				}
			}
		})
	}
}

func TestRecheckError(t *testing.T) {
	boom := errors.New("boom")
	for _, cached := range []bool{false, true} {
		t.Run(strconv.FormatBool(cached), func(t *testing.T) {
			s := NewShelf[page](newMemCatalog(), "page", 1)
			_ = s.Save("k", Entry[page]{ETag: `"v1"`, FetchedAt: time.Now().Add(-time.Hour)})
			c := New[page]()
			if cached {
				s.Warm(c, "k", false)
			}
			srv := answer{err: boom}
			if _, got, err := s.Recheck(t.Context(), c, "k", srv.fetch); !errors.Is(err, boom) || got != RecheckSkipped {
				t.Errorf("Recheck = %v, %v; want %v", got, err, boom)
			}
		})
	}
	var none *Shelf[page]
	if _, got, err := none.Recheck(t.Context(), New[page](), "k", (&answer{}).fetch); got != RecheckSkipped || err != nil {
		t.Errorf("Recheck of a nil shelf = %v, %v", got, err)
	}
}

func TestRecheckJoinsFetch(t *testing.T) {
	s := NewShelf[page](newMemCatalog(), "page", 1)
	_ = s.Save("k", Entry[page]{ETag: `"v1"`, FetchedAt: time.Now().Add(-time.Hour)})
	c := New[page]()
	s.Warm(c, "k", false)
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.Fetch(context.Background(), "k", func(context.Context, Entry[page], bool) (Entry[page], error) {
			close(started)
			<-release
			return Entry[page]{Value: page{Next: "read"}, ETag: `"v2"`}, nil
		})
	}()
	<-started
	srv := answer{etag: `"v1"`}
	go func() { close(release) }()
	e, got, err := s.Recheck(t.Context(), c, "k", srv.fetch)
	<-done
	if err != nil || got != RecheckSkipped || e.Value.Next != "read" || len(srv.calls) != 0 {
		t.Errorf("Recheck = %+v, %v, %v, asked %q; want the running fetch's entry, skipped", e, got, err, srv.calls)
	}
}
