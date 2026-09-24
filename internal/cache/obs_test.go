package cache

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
)

// freshStats makes the stats of package obs empty for the rest of the test.
func freshStats(t *testing.T) *obs.Stats {
	t.Helper()
	s := obs.NewStats()
	prev := obs.SetDefault(s)
	t.Cleanup(func() { obs.SetDefault(prev) })
	return s
}

func cacheSummary(s *obs.Stats, kind string) obs.CacheSummary {
	for _, c := range s.Summary().Cache {
		if c.Kind == kind {
			return c
		}
	}
	return obs.CacheSummary{}
}

func TestFetchCounts(t *testing.T) {
	s := freshStats(t)
	c := New[int](WithCapacity(2))
	ctx := t.Context()
	fetch := func(_ context.Context, _ Entry[int], ok bool) (Entry[int], error) {
		if ok {
			return Entry[int]{}, ErrNotModified
		}
		return Entry[int]{Value: 1}, nil
	}
	for range 3 {
		if _, err := c.Fetch(ctx, "pulls:a", fetch); err != nil {
			t.Fatal(err)
		}
	}
	c.Invalidate("pulls:a")
	if _, err := c.Fetch(ctx, "pulls:a", fetch); err != nil {
		t.Fatal(err)
	}
	// A full cache evicts.
	c.Set("pulls:b", Entry[int]{})
	c.Set("pulls:c", Entry[int]{})
	if !c.Seed("issue:x", Entry[int]{}) {
		t.Fatal("Seed didn't seed")
	}

	want := obs.CacheSummary{Kind: "pulls", Hit: 2, Miss: 2, HitRatio: 0.5, Revalidated: 1, Evicted: 2}
	if got := cacheSummary(s, "pulls"); got != want {
		t.Errorf("pulls = %+v, want %+v", got, want)
	}
	if got := cacheSummary(s, "issue"); got.Seeded != 1 {
		t.Errorf("issue = %+v, want one seeded", got)
	}
}

func TestFetchCountsShared(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := freshStats(t)
		c := New[int]()
		release := make(chan struct{})
		for range 2 {
			go func() {
				_, _ = c.Fetch(context.Background(), "k", blockUntil(release, 1, new(atomic.Int32)))
			}()
		}
		synctest.Wait()
		close(release)
		synctest.Wait()
		if got := cacheSummary(s, "other"); got.Miss != 1 || got.Shared != 1 {
			t.Errorf("summary = %+v, want a miss and a shared read", got)
		}
	})
}

func TestFetchLogsAtDebug(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(obs.NewLogger(&buf, slog.LevelDebug, "s_test"))
	t.Cleanup(func() { slog.SetDefault(prev) })

	c := New[int]()
	ctx := obs.WithTrace(t.Context(), "open.pull")
	for range 2 {
		if _, err := c.Fetch(ctx, "pull:cli/cli#1", value(1, new(atomic.Int32))); err != nil {
			t.Fatal(err)
		}
	}
	id, _ := obs.TraceID(ctx)
	out := buf.String()
	for _, want := range []string{`"found":"miss"`, `"found":"hit"`, `"kind":"pull"`, `"span":"cache.memory"`, `"trace_id":"` + id + `"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %s:\n%s", want, out)
		}
	}
}

func TestWarmCountsStaleServed(t *testing.T) {
	s := freshStats(t)
	store := newMemStore()
	shelf := NewShelf[int](store, "pulllist", 1)
	if err := shelf.Save("pulls:a", Entry[int]{Value: 1, FetchedAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	c := New[int]()
	if _, ok := shelf.Warm(c, "pulls:a"); !ok {
		t.Fatal("Warm didn't serve the stale entry")
	}
	if got := cacheSummary(s, "pulls"); got.Seeded != 1 || got.StaleServed != 1 {
		t.Errorf("summary = %+v, want one seeded and served stale", got)
	}
}

func TestKindOf(t *testing.T) {
	for key, want := range map[string]string{
		"pulls:cli/cli?state=open": "pulls",
		"repo/cli/cli":             "repo",
		"notifications?all=false":  "notifications",
		"k":                        "other",
		":odd":                     "other",
	} {
		if got := kindOf(key); got != want {
			t.Errorf("kindOf(%q) = %q, want %q", key, got, want)
		}
	}
}
