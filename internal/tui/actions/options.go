package actions

import (
	"context"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Option configures a Modal in [New].
type Option func(*options)

// Follow starts polling run runID of repo while it runs, so that its
// changes arrive as ui.SyncMsg under actions.RunSyncKey, and returns the
// function that stops it. It must not block.
type Follow func(repo core.RepoRef, runID int64) (stop func())

// Viewer returns the login of the signed-in user, for the runs they
// started. It may do I/O.
type Viewer func(ctx context.Context) (string, error)

type options struct {
	offline *ui.Offline
	icons   ui.Icons
	follow  Follow
	viewer  Viewer
	repos   ui.Repos
	filter  core.RunFilter
	// voice words the errors of the runs and the log; New makes one of
	// its keys if it is nil.
	voice *ui.Voice
	// rest is how long the cursor rests on a run or a job before its jobs
	// or its log are read; tests set 0.
	rest time.Duration
	// tick is how often the timers of what runs move on; tests set 0,
	// which stops them.
	tick time.Duration
	now  func() time.Time
}

// defaultRest keeps a scroll through the runs from reading every one of
// them.
const defaultRest = 150 * time.Millisecond

func defaultOptions() options {
	return options{offline: new(ui.Offline), icons: ui.NewIcons(""), rest: defaultRest, tick: time.Second, now: time.Now}
}

// WithOffline shares off with the sections, so that the user is told once
// for all of them that GitHub can't be reached. By default the modal has
// its own.
func WithOffline(off *ui.Offline) Option {
	return func(o *options) {
		if off != nil {
			o.offline = off
		}
	}
}

// WithVoice sets how the modal words what went wrong, with the keys a hint
// names and the log it points to. By default the hints name the configured
// keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(o *options) { o.voice = &v }
}

// WithIcons sets the glyphs of the states of runs, jobs and steps. The
// default is the Nerd Font set.
func WithIcons(ic ui.Icons) Option {
	return func(o *options) { o.icons = ic }
}

// WithFollow sets how the modal follows a run in progress. Without it, a
// run in progress changes only when it is read again.
func WithFollow(f Follow) Option {
	return func(o *options) { o.follow = f }
}

// WithFilter opens the modal on the runs that f selects, such as those of
// a branch that failed. The default shows every run.
func WithFilter(f core.RunFilter) Option {
	return func(o *options) { o.filter = f }
}

// WithViewer sets how the modal learns who the user is, for the Mine tab
// and the @me of the filter. Without it they show every run.
func WithViewer(v Viewer) Option {
	return func(o *options) { o.viewer = v }
}

// WithRepos reads what the viewer may do in the repository from r, so
// that re-runs and cancels are offered only with write access. Until it
// is known, or without it, they are offered, and GitHub refuses what it
// doesn't allow.
func WithRepos(r ui.Repos) Option {
	return func(o *options) { o.repos = r }
}
