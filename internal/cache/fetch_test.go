package cache

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// value returns a FetchFunc that always succeeds with v and counts its calls.
func value(v int, calls *atomic.Int32) FetchFunc[int] {
	return func(context.Context, Entry[int], bool) (Entry[int], error) {
		calls.Add(1)
		return Entry[int]{Value: v}, nil
	}
}

// blockUntil is like value, but waits for release to be closed first.
func blockUntil(release <-chan struct{}, v int, calls *atomic.Int32) FetchFunc[int] {
	return func(context.Context, Entry[int], bool) (Entry[int], error) {
		calls.Add(1)
		<-release
		return Entry[int]{Value: v}, nil
	}
}

func TestFetchMiss(t *testing.T) {
	c := New[int]()
	var gotOK bool
	e, err := c.Fetch(t.Context(), "k", func(_ context.Context, _ Entry[int], ok bool) (Entry[int], error) {
		gotOK = ok
		return Entry[int]{Value: 1, ETag: `"v1"`}, nil
	})
	if err != nil || e.Value != 1 {
		t.Fatalf("Fetch = %d, %v; want 1, nil", e.Value, err)
	}
	if gotOK {
		t.Error("fn got ok = true on a miss")
	}
	if e.FetchedAt.IsZero() {
		t.Error("returned FetchedAt is zero")
	}
	if got, st := c.Get("k"); st != Fresh || got.ETag != `"v1"` {
		t.Errorf("Get = %+v, %v; want stored fresh entry", got, st)
	}
}

func TestFetchFreshHit(t *testing.T) {
	c := New[int]()
	c.Set("k", Entry[int]{Value: 1})
	var calls atomic.Int32
	e, err := c.Fetch(t.Context(), "k", value(2, &calls))
	if err != nil || e.Value != 1 {
		t.Errorf("Fetch = %d, %v; want cached 1, nil", e.Value, err)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("fn ran %d times on a fresh hit, want 0", n)
	}
}

func TestFetchStalePassesPrevious(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New[int](WithTTL(time.Minute))
		c.Set("k", Entry[int]{Value: 1, ETag: `"v1"`})
		time.Sleep(time.Minute)

		var prev Entry[int]
		var ok bool
		e, err := c.Fetch(t.Context(), "k", func(_ context.Context, p Entry[int], o bool) (Entry[int], error) {
			prev, ok = p, o
			return Entry[int]{Value: 2, ETag: `"v2"`}, nil
		})
		if err != nil || e.Value != 2 {
			t.Fatalf("Fetch = %d, %v; want 2, nil", e.Value, err)
		}
		if !ok || prev.ETag != `"v1"` {
			t.Errorf("fn got prev = %+v, ok = %v; want the stale entry", prev, ok)
		}
		if got, st := c.Get("k"); st != Fresh || got.Value != 2 {
			t.Errorf("Get = %d, %v; want 2, fresh", got.Value, st)
		}
	})
}

func TestFetchNotModified(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New[int](WithTTL(time.Minute))
		c.Set("k", Entry[int]{Value: 1, ETag: `"v1"`, Tags: []string{"t"}})
		time.Sleep(time.Hour)

		e, err := c.Fetch(t.Context(), "k", func(context.Context, Entry[int], bool) (Entry[int], error) {
			return Entry[int]{}, ErrNotModified
		})
		if err != nil {
			t.Fatalf("Fetch error = %v", err)
		}
		if e.Value != 1 || e.ETag != `"v1"` || len(e.Tags) != 1 {
			t.Errorf("Fetch = %+v, want the previous entry", e)
		}
		if !e.FetchedAt.Equal(time.Now()) {
			t.Errorf("FetchedAt = %v, want renewed to %v", e.FetchedAt, time.Now())
		}
		if _, st := c.Get("k"); st != Fresh {
			t.Errorf("state after not modified = %v, want fresh", st)
		}
	})
}

func TestFetchNotModifiedWithoutEntry(t *testing.T) {
	c := New[int]()
	_, err := c.Fetch(t.Context(), "k", func(context.Context, Entry[int], bool) (Entry[int], error) {
		return Entry[int]{}, ErrNotModified
	})
	if !errors.Is(err, ErrNotModified) {
		t.Errorf("Fetch error = %v, want ErrNotModified", err)
	}
	if c.Len() != 0 {
		t.Errorf("Len() = %d, want 0", c.Len())
	}
}

func TestFetchErrorNotCached(t *testing.T) {
	c := New[int]()
	c.Set("k", Entry[int]{Value: 1})
	c.Invalidate("k")

	errBoom := errors.New("boom")
	var calls atomic.Int32
	fail := func(context.Context, Entry[int], bool) (Entry[int], error) {
		calls.Add(1)
		return Entry[int]{}, errBoom
	}
	for range 2 {
		if _, err := c.Fetch(t.Context(), "k", fail); !errors.Is(err, errBoom) {
			t.Errorf("Fetch error = %v, want %v", err, errBoom)
		}
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("fn ran %d times, want 2 since errors aren't cached", n)
	}
	if e, st := c.Get("k"); st != Stale || e.Value != 1 {
		t.Errorf("Get = %d, %v; want the stale entry to survive the error", e.Value, st)
	}
}

func TestFetchDedupes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New[int]()
		release := make(chan struct{})
		var calls atomic.Int32
		fn := blockUntil(release, 7, &calls)

		const n = 10
		results := make([]int, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() {
				e, err := c.Fetch(t.Context(), "k", fn)
				if err != nil {
					t.Errorf("Fetch error = %v", err)
				}
				results[i] = e.Value
			})
		}
		synctest.Wait()
		close(release)
		wg.Wait()

		if got := calls.Load(); got != 1 {
			t.Errorf("fn ran %d times for %d callers, want 1", got, n)
		}
		for i, v := range results {
			if v != 7 {
				t.Errorf("caller %d got %d, want 7", i, v)
			}
		}
	})
}

func TestFetchContextCanceled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New[int]()
		var fnErr error
		block := func(ctx context.Context, _ Entry[int], _ bool) (Entry[int], error) {
			<-ctx.Done()
			fnErr = ctx.Err()
			return Entry[int]{Value: 1}, nil
		}

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		_, err := c.Fetch(ctx, "k", block)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Fetch error = %v, want deadline exceeded", err)
		}
		synctest.Wait()
		if !errors.Is(fnErr, context.Canceled) {
			t.Errorf("fn context error = %v, want canceled once the only caller left", fnErr)
		}
		if c.Len() != 0 {
			t.Error("an abandoned fetch stored its result")
		}

		var calls atomic.Int32
		if e, err := c.Fetch(t.Context(), "k", value(2, &calls)); err != nil || e.Value != 2 {
			t.Errorf("Fetch after cancel = %d, %v; want a new call returning 2", e.Value, err)
		}
	})
}

func TestFetchCanceledBeforeStart(t *testing.T) {
	c := New[int]()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var calls atomic.Int32
	if _, err := c.Fetch(ctx, "k", value(1, &calls)); !errors.Is(err, context.Canceled) {
		t.Errorf("Fetch error = %v, want canceled", err)
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("fn ran %d times, want 0", n)
	}
}

func TestFetchSurvivesOneCallerCanceling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New[int]()
		release := make(chan struct{})
		var fnCtx context.Context
		wrapped := blockUntil(release, 1, new(atomic.Int32))
		fn := func(ctx context.Context, prev Entry[int], ok bool) (Entry[int], error) {
			fnCtx = ctx
			return wrapped(ctx, prev, ok)
		}

		ctx, cancel := context.WithCancel(t.Context())
		var first error
		done := make(chan struct{})
		go func() {
			defer close(done)
			_, first = c.Fetch(ctx, "k", fn)
		}()
		synctest.Wait()

		var second Entry[int]
		var secondErr error
		go func() { second, secondErr = c.Fetch(t.Context(), "k", fn) }()
		synctest.Wait()

		cancel()
		<-done
		if !errors.Is(first, context.Canceled) {
			t.Errorf("first Fetch error = %v, want canceled", first)
		}
		if err := fnCtx.Err(); err != nil {
			t.Errorf("fn context error = %v while a caller is still waiting", err)
		}

		close(release)
		synctest.Wait()
		if secondErr != nil || second.Value != 1 {
			t.Errorf("second Fetch = %d, %v; want 1, nil", second.Value, secondErr)
		}
	})
}

func TestFetchKeepsNewerWrite(t *testing.T) {
	tests := []struct {
		name  string
		write func(c *Cache[int])
		value int
		want  State
	}{
		{"set", func(c *Cache[int]) { c.Set("k", Entry[int]{Value: 2}) }, 2, Fresh},
		{"mutate", func(c *Cache[int]) { c.Mutate("k", func(v int) int { return v }) }, 2, Stale},
		// An invalidation isn't a newer value: the fetched one replaces the
		// old, but stays stale, as it may predate what was invalidated.
		{"invalidate", func(c *Cache[int]) { c.Invalidate("k") }, 1, Stale},
		{"invalidate tag", func(c *Cache[int]) { c.InvalidateTag("t") }, 1, Stale},
		{"invalidate then set", func(c *Cache[int]) {
			c.Invalidate("k")
			c.Set("k", Entry[int]{Value: 2})
		}, 2, Fresh},
		{"mutate then roll back", func(c *Cache[int]) {
			rollback, _ := c.Mutate("k", func(v int) int { return v + 5 })
			rollback()
		}, 1, Fresh},
		{"invalidate then mutate", func(c *Cache[int]) {
			c.Invalidate("k")
			c.Mutate("k", func(v int) int { return v + 5 })
		}, 7, Stale},
		{"mutate then invalidate", func(c *Cache[int]) {
			c.Mutate("k", func(v int) int { return v + 5 })
			c.Invalidate("k")
		}, 7, Stale},
		// The rollback finds the entry invalidated since, so it can only
		// mark it stale, and the next Fetch reads it again.
		{"mutate, invalidate, roll back", func(c *Cache[int]) {
			rollback, _ := c.Mutate("k", func(v int) int { return v + 5 })
			c.Invalidate("k")
			rollback()
		}, 7, Stale},
		{"invalidate, mutate, roll back", func(c *Cache[int]) {
			c.Invalidate("k")
			rollback, _ := c.Mutate("k", func(v int) int { return v + 5 })
			rollback()
		}, 1, Stale},
		{"mutate tag without a change, then invalidate", func(c *Cache[int]) {
			c.MutateTag("t", func(v int) (int, bool) { return v, false })
			c.Invalidate("k")
		}, 1, Stale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c := New[int](WithTTL(time.Minute))
				c.Set("k", Entry[int]{Value: 2, Tags: []string{"t"}})
				time.Sleep(time.Minute)

				release := make(chan struct{})
				var got Entry[int]
				go func() { got, _ = c.Fetch(t.Context(), "k", blockUntil(release, 1, new(atomic.Int32))) }()
				synctest.Wait()
				tt.write(c)
				close(release)
				synctest.Wait()

				if got.Value != 1 {
					t.Errorf("Fetch returned %d, want 1", got.Value)
				}
				if e, st := c.Get("k"); e.Value != tt.value || st != tt.want {
					t.Errorf("Get = %d, %v; want %d, %v", e.Value, st, tt.value, tt.want)
				}
			})
		})
	}
}

// TestFetchNotModifiedKeepsNewerWrite checks what a 304 keeps when the
// entry changed while it was asked for: the entry it confirmed, stale, after
// an invalidation, and the newer value after a write.
func TestFetchNotModifiedKeepsNewerWrite(t *testing.T) {
	tests := []struct {
		name  string
		write func(c *Cache[int])
		value int
		want  State
	}{
		{"nothing", func(*Cache[int]) {}, 2, Fresh},
		{"invalidate", func(c *Cache[int]) { c.Invalidate("k") }, 2, Stale},
		{"mutate", func(c *Cache[int]) { c.Mutate("k", func(v int) int { return v + 5 }) }, 7, Stale},
		{"set", func(c *Cache[int]) { c.Set("k", Entry[int]{Value: 3}) }, 3, Fresh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c := New[int](WithTTL(time.Minute))
				c.Set("k", Entry[int]{Value: 2, ETag: `"e"`})
				time.Sleep(time.Minute)

				release := make(chan struct{})
				var got Entry[int]
				go func() {
					got, _ = c.Fetch(t.Context(), "k", func(context.Context, Entry[int], bool) (Entry[int], error) {
						<-release
						return Entry[int]{}, ErrNotModified
					})
				}()
				synctest.Wait()
				tt.write(c)
				close(release)
				synctest.Wait()

				if got.Value != 2 {
					t.Errorf("Fetch returned %d, want the confirmed 2", got.Value)
				}
				if e, st := c.Get("k"); e.Value != tt.value || st != tt.want {
					t.Errorf("Get = %d, %v; want %d, %v", e.Value, st, tt.value, tt.want)
				}
			})
		})
	}
}

func TestConcurrentFetch(t *testing.T) {
	c := New[int](WithCapacity(8), WithTTL(time.Millisecond))
	fn := func(ctx context.Context, prev Entry[int], ok bool) (Entry[int], error) {
		if err := ctx.Err(); err != nil {
			return Entry[int]{}, err
		}
		if ok && prev.Value%2 == 0 {
			return Entry[int]{}, ErrNotModified
		}
		return Entry[int]{Value: prev.Value + 1}, nil
	}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 500 {
				k := strconv.Itoa((g + i) % 16)
				ctx, cancel := context.WithCancel(t.Context())
				if i%7 == 0 {
					cancel()
				}
				switch i % 4 {
				case 0:
					c.Set(k, Entry[int]{Value: i})
				case 1:
					c.Invalidate(k)
				default:
					_, _ = c.Fetch(ctx, k, fn)
				}
				cancel()
			}
		})
	}
	wg.Wait()
	if got := c.Len(); got > 8 {
		t.Errorf("Len() = %d, want at most 8", got)
	}
}
