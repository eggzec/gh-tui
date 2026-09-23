package issues

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/watch"
)

func TestSyncKey(t *testing.T) {
	if got := SyncKey(core.RepoRef{Owner: "Octo-Org", Name: "Hello"}); got != "issues:octo-org/hello" {
		t.Errorf("SyncKey = %q, want %q", got, "issues:octo-org/hello")
	}
}

func TestPoll(t *testing.T) {
	// The polls and reads run one at a time, so the fakes need no lock.
	etag := `"p1"`
	var probed []string
	api := &fakeAPI{
		t: t,
		listIssues: func(_ core.StateFilter, _ string, _ int, c github.Conditional) (core.Page[core.Issue], github.Response, error) {
			if c.ETag != "" {
				return core.Page[core.Issue]{}, notModified, nil
			}
			return page("", 7), ok(`"l1"`), nil
		},
		probe: func(c github.Conditional) (github.Response, error) {
			probed = append(probed, c.ETag)
			if c.ETag == etag {
				res := notModified
				res.PollInterval = time.Minute
				return res, nil
			}
			res := ok(etag)
			res.PollInterval = time.Minute
			return res, nil
		},
	}
	s := New(api)
	q := ListQuery{Repo: repo}
	if _, err := s.List(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	api.checkCalls(t, "ListIssues")
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
	api.checkCalls(t, "ProbeIssues", "ProbeIssues")
	if _, err := s.List(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	api.checkCalls(t) // Still fresh.

	etag = `"p2"`
	check("new ETag", watch.Result{Changed: true, Interval: time.Minute})
	api.checkCalls(t, "ProbeIssues")
	if _, err := s.List(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	api.checkCalls(t, "ListIssues") // Revalidated, and answered with a 304.
	check("304 after the change", watch.Result{Interval: time.Minute})
	api.checkCalls(t, "ProbeIssues")

	want := []string{"", `"p1"`, `"p1"`, `"p2"`}
	if strings.Join(probed, " ") != strings.Join(want, " ") {
		t.Errorf("probes sent ETags %q, want %q", probed, want)
	}
}

func TestPollError(t *testing.T) {
	api := &fakeAPI{t: t, probe: func(github.Conditional) (github.Response, error) {
		return github.Response{}, core.ErrRateLimited
	}}
	_, err := New(api).Poll(repo)(t.Context())
	if !errors.Is(err, core.ErrRateLimited) {
		t.Fatalf("error = %v, want %v", err, core.ErrRateLimited)
	}
	if want := "poll issues of octo-org/hello: "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want it to start with %q", err, want)
	}
}
