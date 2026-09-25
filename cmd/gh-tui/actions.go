package main

import (
	"context"
	"errors"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/actions"
	"github.com/eggzec/gh-tui/internal/tui/checks"
	"github.com/eggzec/gh-tui/internal/watch"
)

// runPollInterval is how often a run in progress is polled while the
// Actions modal shows it: its steps move by the second, and each poll is
// two conditional requests, free while nothing moved.
const runPollInterval = 10 * time.Second

// followRuns returns how the Actions modal follows a run in progress: it
// subscribes the run's poll, which starts at once, and then asks every
// runPollInterval, or as often as GitHub asks.
func followRuns(subscribe func(key string, fn watch.PollFunc) func(), refresh func(key string),
	poll func(repo core.RepoRef, runID int64) watch.PollFunc,
) actions.Follow {
	return func(repo core.RepoRef, runID int64) func() {
		key := actionssvc.RunSyncKey(repo, runID)
		fn := poll(repo, runID)
		stop := subscribe(key, func(ctx context.Context) (watch.Result, error) {
			res, err := fn(ctx)
			if res.Interval == 0 {
				res.Interval = runPollInterval
			}
			return res, err
		})
		refresh(key)
		return stop
	}
}

// checksPollInterval is how often the checks of a pull request are
// polled while its Checks step shows some pending: each poll is a GraphQL
// query, which costs a point of the rate limit.
const checksPollInterval = 15 * time.Second

// watchChecks returns how the Checks step polls the checks of a pull
// request while some are pending: it subscribes their poll, which starts at
// once, and then asks every checksPollInterval.
func watchChecks(subscribe func(key string, fn watch.PollFunc) func(), refresh func(key string),
	poll func(q actionssvc.ChecksQuery) watch.PollFunc,
) checks.Watch {
	return func(q actionssvc.ChecksQuery) func() {
		key := actionssvc.ChecksSyncKey(q)
		fn := poll(q)
		stop := subscribe(key, func(ctx context.Context) (watch.Result, error) {
			res, err := fn(ctx)
			if res.Interval == 0 {
				res.Interval = checksPollInterval
			}
			return res, err
		})
		refresh(key)
		return stop
	}
}

var errNoLogin = errors.New("the header has no login")

// viewerLogin returns the login of the signed-in user, from the header the
// dashboard read, even a stale one, since a login doesn't change, or else
// by reading it.
func viewerLogin(cached func() (core.Header, bool), read func(ctx context.Context) (core.Header, error)) actions.Viewer {
	return func(ctx context.Context) (string, error) {
		if h, ok := cached(); ok && h.Profile.Login != "" {
			return h.Profile.Login, nil
		}
		h, err := read(ctx)
		if err != nil {
			return "", err
		}
		if h.Profile.Login == "" {
			return "", errNoLogin
		}
		return h.Profile.Login, nil
	}
}
