package main

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
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

// TestWatchChecksPollsWhilePending runs the checks' poll on the sync
// engine with a fake clock: at once, then every checksPollInterval, until
// the checks are done or the step stops it.
func TestWatchChecksPollsWhilePending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := watch.New(watch.WithInterval(time.Minute))
		var mu sync.Mutex
		polls := 0
		poll := func(actionssvc.ChecksQuery) watch.PollFunc {
			return func(context.Context) (watch.Result, error) {
				mu.Lock()
				defer mu.Unlock()
				polls++
				return watch.Result{Changed: true}, nil
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { _ = e.Run(ctx); close(done) }()
		var events []string
		go func() {
			for ev := range e.Events() {
				mu.Lock()
				events = append(events, ev.Key)
				mu.Unlock()
			}
		}()
		q := actionssvc.ChecksQuery{Repo: core.RepoRef{Owner: "o", Name: "r"}, Number: 5}
		stop := watchChecks(e.Subscribe, e.Refresh, poll)(q)
		synctest.Wait()
		time.Sleep(2*checksPollInterval + time.Second)
		synctest.Wait()
		mu.Lock()
		got, keys := polls, slices.Clone(events)
		mu.Unlock()
		if got != 3 {
			t.Errorf("polled %d times in %v, want at once and every %v", got, 2*checksPollInterval, checksPollInterval)
		}
		if len(keys) == 0 || keys[0] != actionssvc.ChecksSyncKey(q) {
			t.Errorf("events %v, want the checks' sync key", keys)
		}
		stop()
		time.Sleep(5 * checksPollInterval)
		synctest.Wait()
		mu.Lock()
		if polls != got {
			t.Errorf("polled %d times after stop, want none", polls-got)
		}
		mu.Unlock()
		cancel()
		<-done
	})
}
