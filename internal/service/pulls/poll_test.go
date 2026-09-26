package pulls

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/watch"
)

func TestSyncKey(t *testing.T) {
	if got := SyncKey(core.RepoRef{Owner: "EggZec", Name: "GH-TUI"}); got != "pulls:eggzec/gh-tui" {
		t.Errorf("SyncKey = %q, want %q", got, "pulls:eggzec/gh-tui")
	}
}

func TestPoll(t *testing.T) {
	// The polls run one at a time, so the fake needs no lock.
	etag := `"e1"`
	var conds []github.Conditional
	api := &fakeAPI{list: listing}
	api.probe = func(_ context.Context, r core.RepoRef, cond github.Conditional) (github.Response, error) {
		if r != repo {
			t.Errorf("probed %v, want %v", r, repo)
		}
		conds = append(conds, cond)
		res := github.Response{PollInterval: time.Minute}
		if cond.ETag == etag {
			res.NotModified = true
			return res, nil
		}
		res.ETag = etag
		return res, nil
	}
	s := New(api)
	q := ListQuery{Repo: repo, State: core.StateOpen}
	if _, err := s.List(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	poll := s.Poll(repo)
	check := func(step string, want watch.Result) {
		t.Helper()
		got, err := poll(t.Context())
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if got != want {
			t.Errorf("%s: Poll = %+v, want %+v", step, got, want)
		}
	}

	check("first", watch.Result{Interval: time.Minute})
	check("304", watch.Result{Interval: time.Minute})
	if api.count("list") != 1 || api.count("get") != 0 {
		t.Errorf("GraphQL reads while nothing changed: list %d, get %d", api.count("list"), api.count("get"))
	}
	if _, err := s.List(t.Context(), q); err != nil || api.count("list") != 1 {
		t.Errorf("List refetched after unchanged polls (%d calls, %v), want a fresh hit", api.count("list"), err)
	}

	etag = `"e2"`
	check("new ETag", watch.Result{Changed: true, Interval: time.Minute})
	if _, ok := s.CachedList(q); !ok {
		t.Error("list page was dropped, want it kept stale")
	}
	if _, err := s.List(t.Context(), q); err != nil || api.count("list") != 2 {
		t.Errorf("List after a change made %d calls (%v), want it to reach GitHub", api.count("list"), err)
	}
	check("304 after the change", watch.Result{Interval: time.Minute})

	// The list read after the change probes first, with Poll's ETag.
	want := []string{"", `"e1"`, `"e1"`, `"e2"`, `"e2"`}
	if len(conds) != len(want) {
		t.Fatalf("probed %d times, want %d", len(conds), len(want))
	}
	for i, c := range conds {
		if c.ETag != want[i] {
			t.Errorf("probe %d sent ETag %q, want %q", i, c.ETag, want[i])
		}
	}
}

func TestPollError(t *testing.T) {
	api := &fakeAPI{}
	api.probe = func(context.Context, core.RepoRef, github.Conditional) (github.Response, error) {
		return github.Response{}, core.ErrRateLimited
	}
	_, err := New(api).Poll(repo)(t.Context())
	if !errors.Is(err, core.ErrRateLimited) {
		t.Fatalf("error = %v, want %v", err, core.ErrRateLimited)
	}
	if want := "poll pulls of eggzec/gh-tui: "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want it to start with %q", err, want)
	}
}
