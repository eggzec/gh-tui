package cache

import (
	"slices"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestGetMiss(t *testing.T) {
	c := New[int]()
	if e, st := c.Get("k"); st != Miss || e.Value != 0 {
		t.Errorf("Get on empty cache = %v, %v; want zero entry, miss", e, st)
	}
}

func TestSetGet(t *testing.T) {
	c := New[string]()
	c.Set("k", Entry[string]{Value: "v", ETag: `"abc"`, Tags: []string{"t"}})
	e, st := c.Get("k")
	if st != Fresh {
		t.Fatalf("state = %v, want fresh", st)
	}
	if e.Value != "v" || e.ETag != `"abc"` {
		t.Errorf("entry = %+v, want value v and etag \"abc\"", e)
	}
	if e.FetchedAt.IsZero() {
		t.Error("FetchedAt is zero, want Set to fill in the current time")
	}
}

func TestLRUEviction(t *testing.T) {
	c := New[int](WithCapacity(3))
	for i, k := range []string{"a", "b", "c"} {
		c.Set(k, Entry[int]{Value: i})
	}
	c.Get("a")               // most recent first: a, c, b
	c.Set("d", Entry[int]{}) // evicts b
	c.Set("c", Entry[int]{}) // refreshes c: c, d, a
	c.Set("e", Entry[int]{}) // evicts a
	c.Set("f", Entry[int]{}) // evicts d
	if got := c.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3", got)
	}
	for k, want := range map[string]State{
		"a": Miss, "b": Miss, "d": Miss,
		"c": Fresh, "e": Fresh, "f": Fresh,
	} {
		if _, st := c.Get(k); st != want {
			t.Errorf("Get(%q) state = %v, want %v", k, st, want)
		}
	}
}

func TestTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New[int](WithTTL(time.Minute))
		c.Set("k", Entry[int]{Value: 1})

		time.Sleep(time.Minute - time.Nanosecond)
		if _, st := c.Get("k"); st != Fresh {
			t.Errorf("just before TTL: state = %v, want fresh", st)
		}
		time.Sleep(time.Nanosecond)
		e, st := c.Get("k")
		if st != Stale {
			t.Errorf("at TTL: state = %v, want stale", st)
		}
		if e.Value != 1 {
			t.Errorf("stale value = %d, want 1", e.Value)
		}

		c.Set("k", Entry[int]{Value: 2})
		if _, st := c.Get("k"); st != Fresh {
			t.Errorf("after Set: state = %v, want fresh", st)
		}
	})
}

func TestSetKeepsFetchedAt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New[int](WithTTL(time.Minute))
		c.Set("k", Entry[int]{FetchedAt: time.Now().Add(-time.Hour)})
		if _, st := c.Get("k"); st != Stale {
			t.Errorf("state = %v, want stale for an old FetchedAt", st)
		}
	})
}

func TestInvalidate(t *testing.T) {
	c := New[int]()
	c.Set("a", Entry[int]{Value: 1})
	c.Set("b", Entry[int]{Value: 2})
	c.Invalidate("a")
	c.Invalidate("missing")

	if e, st := c.Get("a"); st != Stale || e.Value != 1 {
		t.Errorf("Get(a) = %d, %v; want 1, stale", e.Value, st)
	}
	if _, st := c.Get("b"); st != Fresh {
		t.Errorf("Get(b) state = %v, want fresh", st)
	}
	if got := c.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2", got)
	}
}

func TestInvalidateTag(t *testing.T) {
	c := New[int]()
	c.Set("pr/1", Entry[int]{Tags: []string{"repo:a", "pulls"}})
	c.Set("pr/2", Entry[int]{Tags: []string{"repo:b", "pulls"}})
	c.Set("issue/1", Entry[int]{Tags: []string{"repo:a"}})
	c.Set("user", Entry[int]{})

	c.InvalidateTag("pulls")

	for k, want := range map[string]State{
		"pr/1": Stale, "pr/2": Stale, "issue/1": Fresh, "user": Fresh,
	} {
		if _, st := c.Get(k); st != want {
			t.Errorf("Get(%q) state = %v, want %v", k, st, want)
		}
	}
}

func TestInvalidateBefore(t *testing.T) {
	t0 := time.Now()
	c := New[int]()
	c.Set("old", Entry[int]{Tags: []string{"pr/1"}, FetchedAt: t0.Add(-time.Second)})
	c.Set("new", Entry[int]{Tags: []string{"pr/1"}, FetchedAt: t0.Add(time.Second)})
	c.Set("other", Entry[int]{Tags: []string{"pr/2"}, FetchedAt: t0.Add(-time.Second)})

	c.InvalidateBefore("pr/1", t0)

	for k, want := range map[string]State{"old": Stale, "new": Fresh, "other": Fresh} {
		if _, st := c.Get(k); st != want {
			t.Errorf("Get(%q) state = %v, want %v", k, st, want)
		}
	}
}

func TestOptionsIgnoreInvalidValues(t *testing.T) {
	c := New[int](WithCapacity(0), WithTTL(-time.Second))
	if c.opts.capacity != DefaultCapacity || c.opts.ttl != DefaultTTL {
		t.Errorf("options = %+v, want defaults", c.opts)
	}
}

func TestConcurrentAccess(t *testing.T) {
	const capacity = 16
	c := New[int](WithCapacity(capacity))
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 2000 {
				k := strconv.Itoa((g*31 + i) % 64)
				switch i % 5 {
				case 0:
					c.Set(k, Entry[int]{Value: i, Tags: []string{"t" + strconv.Itoa(i%3)}})
				case 1:
					c.Invalidate(k)
				case 2:
					c.InvalidateTag("t1")
				default:
					c.Get(k)
				}
			}
		})
	}
	wg.Wait()
	if got := c.Len(); got > capacity {
		t.Errorf("Len() = %d, want at most %d", got, capacity)
	}
}

func TestTagged(t *testing.T) {
	c := New[int]()
	c.Set("a", Entry[int]{Value: 1, Tags: []string{"repo"}})
	c.Set("b", Entry[int]{Value: 2, Tags: []string{"repo", "other"}})
	c.Set("c", Entry[int]{Value: 3, Tags: []string{"other"}})
	c.Invalidate("b")

	got := c.Tagged("repo")
	slices.Sort(got)
	if !slices.Equal(got, []int{1, 2}) {
		t.Errorf("Tagged(repo) = %v, want [1 2], stale entries included", got)
	}
	if got := c.Tagged("none"); len(got) != 0 {
		t.Errorf("Tagged(none) = %v, want empty", got)
	}
}

func TestMaxSize(t *testing.T) {
	c := New[string](WithCapacity(10), WithMaxSize(10, func(v string) int64 { return int64(len(v)) }))
	c.Set("a", Entry[string]{Value: "aaaa"})
	c.Set("b", Entry[string]{Value: "bbbb"})
	c.Get("a")                                 // most recent first: a, b
	c.Set("c", Entry[string]{Value: "cccc"})   // 12 bytes: evicts b
	c.Set("a", Entry[string]{Value: "a"})      // shrinks a: c, a = 5 bytes
	c.Set("d", Entry[string]{Value: "dddddd"}) // 11 bytes: evicts c
	if got, want := c.Size(), int64(7); got != want {
		t.Errorf("Size() = %d, want %d", got, want)
	}
	for k, want := range map[string]State{"a": Fresh, "b": Miss, "c": Miss, "d": Fresh} {
		if _, st := c.Get(k); st != want {
			t.Errorf("Get(%q) state = %v, want %v", k, st, want)
		}
	}

	// An entry larger than the limit is kept on its own.
	c.Set("big", Entry[string]{Value: "0123456789ab"})
	if _, st := c.Get("big"); st != Fresh || c.Len() != 1 || c.Size() != 12 {
		t.Errorf("after a large Set: state %v, Len %d, Size %d; want only the large entry", st, c.Len(), c.Size())
	}
}

func TestMaxSizeWithCapacity(t *testing.T) {
	c := New[string](WithCapacity(2), WithMaxSize(100, func(v string) int64 { return int64(len(v)) }))
	for _, k := range []string{"a", "b", "c"} {
		c.Set(k, Entry[string]{Value: k + k})
	}
	if c.Len() != 2 || c.Size() != 4 {
		t.Errorf("Len %d, Size %d; want 2 entries of 4 bytes", c.Len(), c.Size())
	}
}

func TestMaxSizeOtherType(t *testing.T) {
	// A size function of another type is ignored rather than panicking.
	c := New[int](WithMaxSize(1, func(v string) int64 { return int64(len(v)) }))
	c.Set("a", Entry[int]{Value: 1})
	c.Set("b", Entry[int]{Value: 2})
	if c.Len() != 2 || c.Size() != 0 {
		t.Errorf("Len %d, Size %d; want 2 unmeasured entries", c.Len(), c.Size())
	}
}

func TestTaggedEntries(t *testing.T) {
	c := New[int]()
	c.Set("a", Entry[int]{Value: 1, ETag: `"a"`, Tags: []string{"t"}})
	c.Set("b", Entry[int]{Value: 2, Tags: []string{"u"}})
	c.Set("c", Entry[int]{Value: 3, Tags: []string{"t", "u"}})
	c.Invalidate("c")
	got := c.TaggedEntries("t")
	if len(got) != 2 || got["a"].Value != 1 || got["a"].ETag != `"a"` || got["c"].Value != 3 {
		t.Errorf("TaggedEntries = %+v, want a and c", got)
	}
}
