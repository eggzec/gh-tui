package fallback

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

type page = core.Page[string]

const key = "list:eggzec/gh-tui"

var (
	old   = cache.Entry[page]{Value: page{Items: []string{"old"}}, ETag: `"v1"`}
	fresh = cache.Entry[page]{Value: page{Items: []string{"new"}}, ETag: `"v2"`}
)

// setup returns a cache and a shelf, holding the old page, stale, if kept
// is set.
func setup(t *testing.T, kept bool) (*cache.Cache[page], *cache.Shelf[page]) {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, shelf := cache.New[page](cache.WithTTL(time.Minute)), cache.NewShelf[page](store, "pages", 1)
	if kept {
		c.Set(key, old)
		c.Invalidate(key)
		if err := shelf.Save(key, old); err != nil {
			t.Fatal(err)
		}
	}
	return c, shelf
}

// serve returns a load that answers each read with the next of answers.
func serve(answers ...func(prev cache.Entry[page]) (cache.Entry[page], error)) cache.FetchFunc[page] {
	return func(_ context.Context, prev cache.Entry[page], _ bool) (cache.Entry[page], error) {
		next := answers[0]
		answers = answers[1:]
		return next(prev)
	}
}

func fail(err error) func(cache.Entry[page]) (cache.Entry[page], error) {
	return func(cache.Entry[page]) (cache.Entry[page], error) { return cache.Entry[page]{}, err }
}

func ok200(cache.Entry[page]) (cache.Entry[page], error) { return fresh, nil }

func ok304(cache.Entry[page]) (cache.Entry[page], error) {
	return cache.Entry[page]{}, cache.ErrNotModified
}

func TestFetchFails(t *testing.T) {
	reset := time.Date(2026, 9, 27, 14, 5, 0, 0, time.UTC)
	tests := []struct {
		kind core.ProblemKind
		err  error
		// served is how the old page is served: offline, limited, or not
		// at all, and dropped whether the shelf drops it.
		served  string
		dropped bool
	}{
		{kind: core.Internal, err: errors.New("decode: unexpected EOF")},
		{kind: core.Canceled, err: context.Canceled},
		{kind: core.Offline, err: fmt.Errorf("%w: dial tcp: connection refused", core.ErrOffline), served: "offline"},
		{kind: core.Unavailable, err: fmt.Errorf("github: 502: %w", core.ErrUnavailable), served: "offline"},
		{kind: core.RateLimited, err: &core.RateLimitError{Reset: reset}, served: "limited"},
		{kind: core.Auth, err: core.ErrUnauthorized, dropped: true},
		{kind: core.Forbidden, err: core.ErrForbidden, dropped: true},
		{kind: core.NotFound, err: core.ErrNotFound, dropped: true},
		{kind: core.Rejected, err: core.ErrConflict},
	}
	for _, tt := range tests {
		if k := core.KindOf(tt.err); k != tt.kind {
			t.Fatalf("KindOf(%v) = %v, want %v", tt.err, k, tt.kind)
		}
		t.Run(tt.kind.String()+" with a kept page", func(t *testing.T) {
			c, shelf := setup(t, true)
			ctx, limited := core.WatchLimit(t.Context())
			e, err := Fetch(ctx, c, shelf, key, Page[string], serve(fail(tt.err)))
			if limited() != (tt.served == "limited") {
				t.Errorf("the watch was told of a limit = %v, want %v", limited(), tt.served == "limited")
			}
			if _, kept := shelf.Load(key); kept == tt.dropped {
				t.Errorf("shelf keeps the page = %v, want %v", kept, !tt.dropped)
			}
			if tt.served == "" {
				if !errors.Is(err, tt.err) {
					t.Errorf("Fetch error = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil || !slices.Equal(e.Value.Items, old.Value.Items) {
				t.Fatalf("Fetch = %+v, %v; want the old page", e.Value, err)
			}
			if e.Value.Offline != (tt.served == "offline") || e.Value.Limited != (tt.served == "limited") {
				t.Errorf("Fetch marked Offline %v, Limited %v; want it %s", e.Value.Offline, e.Value.Limited, tt.served)
			}
			got, st := c.Get(key)
			if st != cache.Stale || got.Value.Offline || got.Value.Limited || !errors.Is(got.Fallback, tt.err) {
				t.Errorf("cache holds %+v, %v; want the old page unmarked, stale, with its Fallback", got, st)
			}
		})
		t.Run(tt.kind.String()+" without a kept page", func(t *testing.T) {
			c, shelf := setup(t, false)
			if _, err := Fetch(t.Context(), c, shelf, key, Page[string], serve(fail(tt.err))); !errors.Is(err, tt.err) {
				t.Errorf("Fetch error = %v, want %v", err, tt.err)
			}
			if c.Len() != 0 {
				t.Errorf("cache holds %d entries, want none", c.Len())
			}
		})
	}
}

// TestFetchTellsLimitUnmarked checks that a value with no mark to set,
// served in place of a read the rate limit refused, is still told to whoever
// watches the read.
func TestFetchTellsLimitUnmarked(t *testing.T) {
	c, shelf := setup(t, true)
	ctx, limited := core.WatchLimit(t.Context())
	e, err := Fetch(ctx, c, shelf, key, None[page], serve(fail(core.ErrRateLimited)))
	if err != nil || !slices.Equal(e.Value.Items, old.Value.Items) {
		t.Fatalf("Fetch = %+v, %v; want the old page", e.Value, err)
	}
	if !limited() {
		t.Error("the watch wasn't told that the read was served for the rate limit")
	}
}

// TestFetchAnsweredAfterFallback checks that any answer after a page was
// served in place of one, a 304 too, serves it unmarked again.
func TestFetchAnsweredAfterFallback(t *testing.T) {
	down := fmt.Errorf("%w: dial tcp: connection refused", core.ErrOffline)
	limited := &core.RateLimitError{Reset: time.Now().Add(time.Hour)}
	tests := []struct {
		name   string
		failed error
		answer func(cache.Entry[page]) (cache.Entry[page], error)
		want   []string
	}{
		{name: "offline, then 304", failed: down, answer: ok304, want: old.Value.Items},
		{name: "offline, then 200", failed: down, answer: ok200, want: fresh.Value.Items},
		{name: "limited, then 304", failed: limited, answer: ok304, want: old.Value.Items},
		{name: "limited, then 200", failed: limited, answer: ok200, want: fresh.Value.Items},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, shelf := setup(t, false)
			var prevs []string
			first := func(cache.Entry[page]) (cache.Entry[page], error) { return old, nil }
			load := serve(first, fail(tt.failed), tt.answer)
			record := func(ctx context.Context, prev cache.Entry[page], ok bool) (cache.Entry[page], error) {
				prevs = append(prevs, prev.ETag)
				return load(ctx, prev, ok)
			}
			// Each read finds the last stale, as a refresh would.
			for range 2 {
				if _, err := Fetch(t.Context(), c, shelf, key, Page[string], record); err != nil {
					t.Fatal(err)
				}
				c.Invalidate(key)
			}
			e, err := Fetch(t.Context(), c, shelf, key, Page[string], record)
			if err != nil || e.Value.Offline || e.Value.Limited || e.Fallback != nil || !slices.Equal(e.Value.Items, tt.want) {
				t.Fatalf("Fetch after the answer = %+v, %v; want %v unmarked", e, err, tt.want)
			}
			// The page served in place of an answer kept its validators.
			if want := []string{"", `"v1"`, `"v1"`}; !slices.Equal(prevs, want) {
				t.Errorf("reads asked with ETags %q, want %q", prevs, want)
			}
			if got, st := c.Get(key); st != cache.Fresh || got.Value.Offline || got.Fallback != nil {
				t.Errorf("cache holds %+v, %v; want the page fresh and unmarked", got, st)
			}
		})
	}
}

func TestFetchUnmarked(t *testing.T) {
	c, shelf := setup(t, true)
	e, err := Fetch(t.Context(), c, shelf, key, None[page], serve(fail(core.ErrOffline)))
	if err != nil || e.Value.Offline || !errors.Is(e.Fallback, core.ErrOffline) {
		t.Errorf("Fetch with None = %+v, %v; want the old page unmarked, with its Fallback", e, err)
	}
}

func TestKeep(t *testing.T) {
	c, shelf := setup(t, true)
	load := Keep(shelf, key, serve(ok304, ok200))
	// A 304 leaves what the shelf keeps, and a 200 keeps the new page.
	for _, want := range []string{old.ETag, fresh.ETag} {
		c.Invalidate(key)
		if _, err := Fetch(t.Context(), c, shelf, key, Page[string], load); err != nil {
			t.Fatal(err)
		}
		if e, ok := shelf.Load(key); !ok || e.ETag != want {
			t.Errorf("shelf keeps %+v, %v; want ETag %s", e, ok, want)
		}
	}
}

// A refusal that drops a kept page logs it, with why, once.
func TestFetchLogsDropped(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(obs.NewLogger(&buf, slog.LevelInfo, "s_test"))
	t.Cleanup(func() { slog.SetDefault(prev) })

	c, shelf := setup(t, true)
	_, _ = Fetch(t.Context(), c, shelf, key, Page[string], serve(fail(core.ErrForbidden)))
	c, shelf = setup(t, false)
	_, _ = Fetch(t.Context(), c, shelf, key, Page[string], serve(fail(core.ErrForbidden)))
	out := buf.String()
	if n := strings.Count(out, `"msg":"kept dropped"`); n != 1 {
		t.Fatalf("%d records, want 1 for the kept page:\n%s", n, out)
	}
	for _, want := range []string{`"level":"INFO"`, `"kind":"pages"`, `"reason":"forbidden"`, `"entry":"` + key + `"`} {
		if !strings.Contains(out, want) {
			t.Errorf("record lacks %s:\n%s", want, out)
		}
	}
}

// TestRefused pins what drops what was kept: GitHub refusing the token or
// the account, or saying the thing isn't there, but not an outage, a rate
// limit, a cancellation, or an action it refused with a reason.
func TestRefused(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"unauthorized", core.ErrUnauthorized, true},
		{"missing scope", &core.ScopeError{Scopes: []string{"repo"}}, true},
		{"forbidden", fmt.Errorf("list: %w", core.ErrForbidden), true},
		{"not found", core.ErrNotFound, true},
		{"offline", core.ErrOffline, false},
		{"rate limited", &core.RateLimitError{}, false},
		{"canceled", context.Canceled, false},
		{"rejected with a reason", &core.RefusedError{Action: "merge", Reason: "not mergeable", Err: core.ErrForbidden}, false},
	}
	for _, tt := range tests {
		if got := Refused(tt.err); got != tt.want {
			t.Errorf("%s: Refused = %v, want %v", tt.name, got, tt.want)
		}
	}
}
