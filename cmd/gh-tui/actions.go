package main

import (
	"context"
	"errors"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/actions"
	"github.com/eggzec/gh-tui/internal/tui/checks"
	"github.com/eggzec/gh-tui/internal/watch"
)

// followRuns returns how the Actions modal follows a run in progress: it
// subscribes the run's poll, which starts at once, and then asks as often
// as subscribe polls runs, or as GitHub asks.
func followRuns(subscribe func(key string, fn watch.PollFunc) func(), refresh func(key string),
	poll func(repo core.RepoRef, runID int64) watch.PollFunc,
) actions.Follow {
	return func(repo core.RepoRef, runID int64) func() {
		key := actionssvc.RunSyncKey(repo, runID)
		stop := subscribe(key, poll(repo, runID))
		refresh(key)
		return stop
	}
}

// watchChecks returns how the Checks step polls the checks of a pull
// request while some are pending: it subscribes their poll, which starts at
// once, and then asks as often as subscribe polls checks.
func watchChecks(subscribe func(key string, fn watch.PollFunc) func(), refresh func(key string),
	poll func(q actionssvc.ChecksQuery) watch.PollFunc,
) checks.Watch {
	return func(q actionssvc.ChecksQuery) func() {
		key := actionssvc.ChecksSyncKey(q)
		stop := subscribe(key, poll(q))
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
