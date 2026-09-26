package cache

import (
	"context"
	"strconv"
	"sync"
	"testing"
)

func inc(v int) int { return v + 1 }

func TestMutate(t *testing.T) {
	c := New[int]()
	c.Set("k", Entry[int]{Value: 1, ETag: `"v1"`, Tags: []string{"t"}})
	if _, ok := c.Mutate("k", inc); !ok {
		t.Fatal("Mutate reported the key as missing")
	}
	e, st := c.Get("k")
	if e.Value != 2 || e.ETag != `"v1"` || len(e.Tags) != 1 || st != Fresh {
		t.Errorf("Get = %+v, %v; want value 2 with the old metadata, fresh", e, st)
	}
}

func TestMutateMissing(t *testing.T) {
	c := New[int]()
	rollback, ok := c.Mutate("k", inc)
	if ok {
		t.Error("Mutate on a missing key reported ok")
	}
	rollback()
	if c.Len() != 0 {
		t.Errorf("Len() = %d, want 0", c.Len())
	}
}

func TestRollbackRestores(t *testing.T) {
	c := New[int]()
	c.Set("fresh", Entry[int]{Value: 1})
	c.Set("stale", Entry[int]{Value: 1})
	c.Invalidate("stale")

	for k, want := range map[string]State{"fresh": Fresh, "stale": Stale} {
		rollback, _ := c.Mutate(k, inc)
		rollback()
		if e, st := c.Get(k); e.Value != 1 || st != want {
			t.Errorf("Get(%q) after rollback = %d, %v; want 1, %v", k, e.Value, st, want)
		}
	}
}

func TestRollbackKeepsNewerWrite(t *testing.T) {
	tests := []struct {
		name  string
		write func(c *Cache[int])
		want  int
	}{
		{"set", func(c *Cache[int]) { c.Set("k", Entry[int]{Value: 10}) }, 10},
		{"invalidate", func(c *Cache[int]) { c.Invalidate("k") }, 2},
		{"invalidate tag", func(c *Cache[int]) { c.InvalidateTag("t") }, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New[int]()
			c.Set("k", Entry[int]{Value: 1, Tags: []string{"t"}})
			rollback, _ := c.Mutate("k", inc)
			tt.write(c)
			rollback()
			if e, st := c.Get("k"); e.Value != tt.want || st != Stale {
				t.Errorf("Get = %d, %v; want %d, stale", e.Value, st, tt.want)
			}
		})
	}
}

func TestRollbackIdempotent(t *testing.T) {
	c := New[int]()
	c.Set("k", Entry[int]{Value: 1})
	rollback, _ := c.Mutate("k", inc)
	rollback()
	c.Set("k", Entry[int]{Value: 10})
	rollback()
	if e, st := c.Get("k"); e.Value != 10 || st != Fresh {
		t.Errorf("Get = %d, %v; want 10, fresh after a second rollback", e.Value, st)
	}
}

func TestRollbackNested(t *testing.T) {
	c := New[int]()
	c.Set("k", Entry[int]{Value: 1})
	undo1, _ := c.Mutate("k", inc)
	undo2, _ := c.Mutate("k", inc)
	undo2()
	if e, _ := c.Get("k"); e.Value != 2 {
		t.Errorf("after inner rollback, value = %d, want 2", e.Value)
	}
	undo1()
	if e, st := c.Get("k"); e.Value != 1 || st != Fresh {
		t.Errorf("after both rollbacks, Get = %d, %v; want 1, fresh", e.Value, st)
	}
}

func TestRollbackAfterEviction(t *testing.T) {
	c := New[int](WithCapacity(1))
	c.Set("k", Entry[int]{Value: 1})
	rollback, _ := c.Mutate("k", inc)
	c.Set("other", Entry[int]{})
	rollback()
	if _, st := c.Get("k"); st != Miss {
		t.Errorf("Get(k) state = %v, want miss; rollback must not re-add evicted keys", st)
	}
	if _, st := c.Get("other"); st != Fresh {
		t.Errorf("Get(other) state = %v, want fresh", st)
	}
}

func TestConcurrentMutate(t *testing.T) {
	c := New[int](WithCapacity(8))
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 1000 {
				k := strconv.Itoa((g + i) % 12)
				switch i % 4 {
				case 0:
					c.Set(k, Entry[int]{Value: i})
				case 1:
					c.InvalidateTag("t")
				default:
					rollback, _ := c.Mutate(k, inc)
					if i%3 == 0 {
						rollback()
						rollback()
					}
				}
			}
		})
	}
	wg.Wait()
	if got := c.Len(); got > 8 {
		t.Errorf("Len() = %d, want at most 8", got)
	}
}

// incIfEven stands in for "update the pages that list this item".
func incIfEven(v int) (int, bool) {
	if v%2 != 0 {
		return v, false
	}
	return v + 1, true
}

func TestMutateTag(t *testing.T) {
	c := New[int]()
	c.Set("a", Entry[int]{Value: 2, Tags: []string{"repo"}})
	c.Set("b", Entry[int]{Value: 3, Tags: []string{"repo"}})
	c.Set("c", Entry[int]{Value: 4, Tags: []string{"other"}})

	rollback := c.MutateTag("repo", incIfEven)
	for key, want := range map[string]int{"a": 3, "b": 3, "c": 4} {
		if e, _ := c.Get(key); e.Value != want {
			t.Errorf("after MutateTag, %s = %d, want %d", key, e.Value, want)
		}
	}

	rollback()
	for key, want := range map[string]int{"a": 2, "b": 3, "c": 4} {
		if e, st := c.Get(key); e.Value != want || st != Fresh {
			t.Errorf("after rollback, %s = %d, %v; want %d, fresh", key, e.Value, st, want)
		}
	}
}

func TestMutateTagLeavesUnchangedEntries(t *testing.T) {
	c := New[int]()
	c.Set("odd", Entry[int]{Value: 3, Tags: []string{"repo"}})
	rollback := c.MutateTag("repo", incIfEven)

	// A newer write to an entry fn left alone must survive the rollback.
	c.Set("odd", Entry[int]{Value: 5, Tags: []string{"repo"}})
	rollback()
	if e, st := c.Get("odd"); e.Value != 5 || st != Fresh {
		t.Errorf("odd = %d, %v; want 5, fresh", e.Value, st)
	}
}

func TestMutateTagRollbackKeepsNewerWrite(t *testing.T) {
	c := New[int]()
	c.Set("a", Entry[int]{Value: 2, Tags: []string{"repo"}})
	c.Set("b", Entry[int]{Value: 4, Tags: []string{"repo"}})
	rollback := c.MutateTag("repo", incIfEven)

	c.Set("a", Entry[int]{Value: 10, Tags: []string{"repo"}})
	rollback()
	rollback()
	if e, st := c.Get("a"); e.Value != 10 || st != Stale {
		t.Errorf("a = %d, %v; want 10, stale", e.Value, st)
	}
	if e, st := c.Get("b"); e.Value != 4 || st != Fresh {
		t.Errorf("b = %d, %v; want 4, fresh", e.Value, st)
	}
}

func TestMutateResizes(t *testing.T) {
	c := New[string](WithMaxSize(100, func(v string) int64 { return int64(len(v)) }))
	c.Set("k", Entry[string]{Value: "ab"})
	rollback, _ := c.Mutate("k", func(v string) string { return v + "cdef" })
	if got := c.Size(); got != 6 {
		t.Errorf("Size after Mutate = %d, want 6", got)
	}
	rollback()
	if got := c.Size(); got != 2 {
		t.Errorf("Size after rollback = %d, want 2", got)
	}
}

// TestFailedChangeNotConfirmed checks that a change that a 304 confirmed
// while it was sent, and that then failed, isn't confirmed again: the
// rollback can't restore the entry, so it drops the validators.
func TestFailedChangeNotConfirmed(t *testing.T) {
	c := New[int]()
	c.Set("k", Entry[int]{Value: 1, ETag: `"e1"`})
	c.Invalidate("k")
	rollback, _ := c.Mutate("k", inc)

	// GitHub still has the value of "e1", and says so to a request that
	// asks with it.
	github := func(_ context.Context, prev Entry[int], _ bool) (Entry[int], error) {
		if prev.ETag == `"e1"` {
			return Entry[int]{}, ErrNotModified
		}
		return Entry[int]{Value: 1, ETag: `"e1"`}, nil
	}
	if e, err := c.Fetch(t.Context(), "k", github); err != nil || e.Value != 2 {
		t.Fatalf("Fetch while the change is sent = %d, %v; want it shown", e.Value, err)
	}
	// The change fails.
	rollback()
	if e, st := c.Get("k"); e.ETag != "" || st != Stale {
		t.Fatalf("after the rollback Get = %+v, %v; want it stale, without validators", e, st)
	}
	if e, err := c.Fetch(t.Context(), "k", github); err != nil || e.Value != 1 || e.ETag != `"e1"` {
		t.Errorf("Fetch after the rollback = %+v, %v; want GitHub's 1 in full", e, err)
	}
}

func TestRollbackKeepsNewerValidators(t *testing.T) {
	c := New[int]()
	c.Set("k", Entry[int]{Value: 1, ETag: `"e1"`})
	rollback, _ := c.Mutate("k", inc)
	c.Set("k", Entry[int]{Value: 5, ETag: `"e2"`})
	rollback()
	if e, st := c.Get("k"); e.Value != 5 || e.ETag != `"e2"` || st != Stale {
		t.Errorf("Get = %+v, %v; want GitHub's newer entry, with its ETag, stale", e, st)
	}
}
