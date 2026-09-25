package main

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/watch"
)

func TestFollowRunsPollsAtOnceAndOften(t *testing.T) {
	subs := &subscriptions{}
	var refreshed []string
	polled := 0
	poll := func(core.RepoRef, int64) watch.PollFunc {
		return func(context.Context) (watch.Result, error) { polled++; return watch.Result{Changed: true}, nil }
	}
	var got watch.Result
	subscribe := func(key string, fn watch.PollFunc) func() {
		got, _ = fn(context.Background())
		return subs.subscribe(key, fn)
	}
	follow := followRuns(subscribe, func(key string) { refreshed = append(refreshed, key) }, poll)
	stop := follow(core.RepoRef{Owner: "o", Name: "r"}, 42)
	stop()
	if want := []string{"+actions:o/r/run/42", "-actions:o/r/run/42"}; !slices.Equal(subs.take(), want) {
		t.Errorf("subscriptions differ from %v", want)
	}
	if !slices.Equal(refreshed, []string{"actions:o/r/run/42"}) {
		t.Errorf("refreshed %v, want the run polled at once", refreshed)
	}
	if polled != 2 || !got.Changed || got.Interval != runPollInterval {
		t.Errorf("polled %d times with %+v, want the change and the run's interval", polled, got)
	}
}

func TestViewerLogin(t *testing.T) {
	reads := 0
	read := func(context.Context) (core.Header, error) {
		reads++
		return core.Header{Profile: core.Profile{Login: "octocat"}}, nil
	}
	kept := func() (core.Header, bool) {
		return core.Header{Profile: core.Profile{Login: "drew"}, Stale: true}, true
	}
	if got, err := viewerLogin(kept, read)(t.Context()); got != "drew" || err != nil || reads != 0 {
		t.Errorf("with a kept header: %q, %v after %d reads", got, err, reads)
	}
	none := func() (core.Header, bool) { return core.Header{}, false }
	if got, err := viewerLogin(none, read)(t.Context()); got != "octocat" || err != nil || reads != 1 {
		t.Errorf("without one: %q, %v after %d reads", got, err, reads)
	}
	boom := errors.New("boom")
	failing := func(context.Context) (core.Header, error) { return core.Header{}, boom }
	if _, err := viewerLogin(none, failing)(t.Context()); !errors.Is(err, boom) {
		t.Errorf("a failed read: %v", err)
	}
}
