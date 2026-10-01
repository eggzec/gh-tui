package recheck

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

var repo = core.RepoRef{Owner: "octo", Name: "hello"}

func shelf(t *testing.T) *cache.Shelf[string] {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return cache.NewShelf[string](store, "thing", 1)
}

func TestEntries(t *testing.T) {
	s := shelf(t)
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	_ = s.Save("a", cache.Entry[string]{ETag: `"a"`, FetchedAt: old})
	_ = s.Save("b", cache.Entry[string]{LastModified: "Thu, 24 Sep 2026 10:00:00 GMT"})
	_ = s.Save("unchecked", cache.Entry[string]{})
	_ = s.Save("unknown", cache.Entry[string]{ETag: `"u"`})

	checked := ""
	got := Entries(s, "thing", 5*time.Minute, func(key string) (Target, bool) {
		if key == "unknown" {
			return Target{}, false
		}
		return Target{Repo: repo, Check: func(context.Context) revalidate.Result {
			checked = key
			return revalidate.Result{}
		}}, true
	})
	slices.SortFunc(got, func(a, b revalidate.Entry) int { return strings.Compare(a.ID, b.ID) })
	if len(got) != 2 || got[0].ID != "thing:a" || got[1].ID != "thing:b" {
		t.Fatalf("Entries = %+v, want a and b, which have validators", got)
	}
	if e := got[0]; e.Repo != repo || !e.CheckedAt.Equal(old) || e.UsedAt.IsZero() || e.FreshFor != 5*time.Minute {
		t.Errorf("entry a = %+v", e)
	}
	got[1].Check(t.Context())
	if checked != "b" {
		t.Errorf("Check of b checked %q", checked)
	}
}

func TestFailure(t *testing.T) {
	reset := time.Date(2026, 9, 24, 11, 0, 0, 0, time.UTC)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want revalidate.Status
	}{
		{"forbidden", t.Context(), fmt.Errorf("get: %w", &github.Error{StatusCode: 403}), revalidate.Gone},
		{"primary limit", t.Context(), fmt.Errorf("get: %w", &core.RateLimitError{Reset: reset}), revalidate.Limited},
		{"network", t.Context(), fmt.Errorf("%w: %w", core.ErrOffline, &url.Error{Op: "Get", URL: "https://api.github.com", Err: errors.New("no route to host")}), revalidate.Offline},
		{"server", t.Context(), &github.Error{StatusCode: 502}, revalidate.Offline},
		{"not found", t.Context(), fmt.Errorf("get: %w", core.ErrNotFound), revalidate.Gone},
		{"gone", t.Context(), fmt.Errorf("get: %w", &github.Error{StatusCode: 410}), revalidate.Gone},
		{"canceled", canceled, &url.Error{Op: "Get", URL: "https://api.github.com", Err: context.Canceled}, revalidate.Failed},
		{"other", t.Context(), errors.New("decode response"), revalidate.Failed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Failure(tt.ctx, tt.err)
			if got.Status != tt.want || !errors.Is(got.Err, tt.err) {
				t.Errorf("Failure = %+v, want %v", got, tt.want)
			}
			if tt.want == revalidate.Limited && !got.RetryAt.Equal(reset) {
				t.Errorf("RetryAt = %v, want %v", got.RetryAt, reset)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	var asked []github.Conditional
	current := `"v2"`
	load := Load(func(_ context.Context, cond github.Conditional) (string, github.Response, error) {
		asked = append(asked, cond)
		if cond.ETag == current {
			return "", github.Response{NotModified: true}, nil
		}
		return "value", github.Response{ETag: current, LastModified: "lm", URL: "https://api.github.com/x"}, nil
	}, func(v string) []string { return []string{"tag:" + v} })

	e, err := load(t.Context(), cache.Entry[string]{}, false)
	want := cache.Entry[string]{Value: "value", ETag: `"v2"`, LastModified: "lm", Source: "https://api.github.com/x", Tags: []string{"tag:value"}}
	if err != nil || e.Value != want.Value || e.ETag != want.ETag || e.Source != want.Source || !slices.Equal(e.Tags, want.Tags) {
		t.Errorf("load = %+v, %v; want %+v", e, err, want)
	}
	if _, err := load(t.Context(), e, true); !errors.Is(err, cache.ErrNotModified) {
		t.Errorf("load with the current ETag = %v, want %v", err, cache.ErrNotModified)
	}
	if want := []github.Conditional{{}, {ETag: `"v2"`, LastModified: "lm"}}; !slices.Equal(asked, want) {
		t.Errorf("asked with %+v, want %+v", asked, want)
	}
}

func TestCheck(t *testing.T) {
	for _, tt := range []struct {
		name string
		etag string
		err  error
		want revalidate.Status
		kept bool
	}{
		{"not modified", `"v1"`, nil, revalidate.NotModified, true},
		{"changed", `"v2"`, nil, revalidate.Changed, true},
		{"refused", "", core.ErrNotFound, revalidate.Gone, false},
		{"offline", "", &github.Error{StatusCode: 503}, revalidate.Offline, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := shelf(t)
			_ = s.Save("k", cache.Entry[string]{Value: "old", ETag: `"v1"`, FetchedAt: time.Now().Add(-time.Hour)})
			c := cache.New[string]()
			load := Load(func(context.Context, github.Conditional) (string, github.Response, error) {
				if tt.err != nil {
					return "", github.Response{}, tt.err
				}
				if tt.etag == `"v1"` {
					return "", github.Response{NotModified: true}, nil
				}
				return "new", github.Response{ETag: tt.etag}, nil
			}, func(string) []string { return nil })
			_, res := Check(t.Context(), c, s, "k", "things:octo/hello", load)
			if res.Status != tt.want {
				t.Errorf("Check = %+v, want %v", res, tt.want)
			}
			if wantSync := tt.want == revalidate.Changed; (res.Sync == "things:octo/hello") != wantSync {
				t.Errorf("Sync = %q, want it only for a change", res.Sync)
			}
			if _, ok := s.Load("k"); ok != tt.kept {
				t.Errorf("kept after the check = %v, want %v", ok, tt.kept)
			}
		})
	}
}
