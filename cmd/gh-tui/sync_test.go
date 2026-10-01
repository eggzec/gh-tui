package main

import (
	"context"
	"maps"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/watch"
)

func TestPollIntervals(t *testing.T) {
	p := config.Poll{Notifications: time.Minute, Lists: 2 * time.Minute, Actions: 10 * time.Second, Checks: 15 * time.Second}
	want := map[watch.Kind]time.Duration{
		pollNotifications: time.Minute, pollLists: 2 * time.Minute, pollActions: 10 * time.Second, pollChecks: 15 * time.Second,
	}
	if got := pollIntervals(p); !maps.Equal(got, want) {
		t.Errorf("pollIntervals = %v, want %v", got, want)
	}
}

// The engine polls each kind of key at its interval of sync.poll, and
// that times sync.unfocused_slowdown while the terminal is unfocused.
func TestNewEngine(t *testing.T) {
	c := config.Sync{
		Poll:              config.Poll{Notifications: 50 * time.Second, Lists: 40 * time.Second, Actions: 10 * time.Second, Checks: 20 * time.Second},
		UnfocusedSlowdown: 3,
	}
	for _, tt := range []struct {
		kind     watch.Kind
		every    time.Duration
		inactive bool
	}{
		{pollNotifications, 50 * time.Second, false},
		{pollLists, 40 * time.Second, false},
		{pollActions, 10 * time.Second, false},
		{pollChecks, 20 * time.Second, false},
		{pollChecks, 60 * time.Second, true},
	} {
		synctest.Test(t, func(t *testing.T) {
			e := newEngine(c)
			e.SetActive(!tt.inactive)
			var (
				mu    sync.Mutex
				times []time.Duration
			)
			start := time.Now()
			subscriber(e, tt.kind)("k", func(context.Context) (watch.Result, error) {
				mu.Lock()
				defer mu.Unlock()
				times = append(times, time.Since(start))
				return watch.Result{}, nil
			})
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() { _ = e.Run(ctx); close(done) }()
			time.Sleep(2*tt.every + time.Second)
			synctest.Wait()
			mu.Lock()
			if want := []time.Duration{tt.every, 2 * tt.every}; !slices.Equal(times, want) {
				t.Errorf("%s (inactive %v) polled at %v, want %v", tt.kind, tt.inactive, times, want)
			}
			mu.Unlock()
			cancel()
			<-done
		})
	}
}

// The revalidator slows down by sync.unfocused_slowdown while the
// terminal is unfocused: its budget of 60 a minute shrinks to 15.
func TestNewRevalidatorSlowsDown(t *testing.T) {
	for _, tt := range []struct {
		slowdown, want int
	}{{1, 40}, {4, 15}} {
		synctest.Test(t, func(t *testing.T) {
			var (
				mu      sync.Mutex
				checked int
			)
			entries := make([]revalidate.Entry, 0, 40)
			for i := range 40 {
				entries = append(entries, revalidate.Entry{ID: string(rune('a' + i)), UsedAt: time.Now(), Check: func(context.Context) revalidate.Result {
					mu.Lock()
					defer mu.Unlock()
					checked++
					return revalidate.Result{Status: revalidate.NotModified}
				}})
			}
			r := newRevalidator(config.Default().Cache, tt.slowdown, func(string) {}, func() []revalidate.Entry { return entries })
			r.SetActive(false)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			go func() { _ = r.Run(ctx) }()
			time.Sleep(30 * time.Second)
			synctest.Wait()
			mu.Lock()
			defer mu.Unlock()
			if checked != tt.want {
				t.Errorf("slowdown %d: checked %d in 30s while unfocused, want %d", tt.slowdown, checked, tt.want)
			}
		})
	}
}
