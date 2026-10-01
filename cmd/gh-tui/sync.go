package main

import (
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/watch"
)

// The kinds of keys the sync engine polls, each at its interval of
// sync.poll.
const (
	pollNotifications watch.Kind = "notifications"
	pollLists         watch.Kind = "lists"
	pollActions       watch.Kind = "actions"
	pollChecks        watch.Kind = "checks"
)

// newEngine returns the sync engine c configures: each kind of key polls
// at its interval, slowed down while the terminal is unfocused.
func newEngine(c config.Sync) *watch.Engine {
	return watch.New(watch.WithIntervals(pollIntervals(c.Poll)), watch.WithIdleMultiplier(c.UnfocusedSlowdown))
}

// pollIntervals returns the sync engine's interval of each kind of key.
func pollIntervals(p config.Poll) map[watch.Kind]time.Duration {
	return map[watch.Kind]time.Duration{
		pollNotifications: p.Notifications,
		pollLists:         p.Lists,
		pollActions:       p.Actions,
		pollChecks:        p.Checks,
	}
}

// subscriber returns how keys of kind are subscribed to e.
func subscriber(e *watch.Engine, kind watch.Kind) func(key string, fn watch.PollFunc) func() {
	return func(key string, fn watch.PollFunc) func() { return e.SubscribeKind(kind, key, fn) }
}
