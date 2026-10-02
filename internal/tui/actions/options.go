package actions

import (
	"context"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
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
	icons  ui.Icons
	follow Follow
	viewer Viewer
	repos  ui.Repos
	filter core.RunFilter
	// dates tell when the runs started.
	dates ui.Dates
	// voice words the errors of the runs and the log; New makes one of
	// its keys if it is nil.
	voice *ui.Voice
	// prefetch is what the modal reads ahead, and how long the cursors
	// rest first; tests set no rest.
	prefetch prefetch
	// tick is how often the timers of what runs move on; tests set 0,
	// which stops them.
	tick time.Duration
	now  func() time.Time
}

func defaultOptions() options {
	d := config.Default()
	return options{icons: ui.NewIcons(d.UI.Icons), prefetch: newPrefetch(d.Prefetch), tick: time.Second, now: time.Now}
}

// prefetch is what the modal reads ahead: the jobs of the runs around the
// cursor of the runs, and the logs of the failed jobs around the cursor
// of the jobs. Their rests also time the reads of the run and the job
// under the cursors, so that a scroll doesn't read each one.
type prefetch struct {
	jobs, logs config.Resolved
}

func newPrefetch(p config.PrefetchLayers) prefetch {
	return prefetch{jobs: ui.Resolve(p, "actions", "jobs"), logs: ui.Resolve(p, "actions", "logs")}
}

// WithPrefetch sets how far the modal reads ahead, and how long a cursor
// rests before it does, as p, the prefetch settings, says for the
// actions. The jobs of the run under the cursor and the log of the job
// under theirs are read once the cursors rest whatever p says, since the
// panes show them. The default is that of config.Default.
func WithPrefetch(p config.PrefetchLayers) Option {
	return func(o *options) { o.prefetch = newPrefetch(p) }
}

// WithVoice sets how the modal words what went wrong, with the keys a hint
// names and the log it points to. By default the hints name the configured
// keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(o *options) { o.voice = &v }
}

// WithDates sets how dates read, as ui.date_format says. The default is
// as ages.
func WithDates(d ui.Dates) Option {
	return func(o *options) { o.dates = d }
}

// WithIcons sets the glyphs of the states of runs, jobs and steps. Without
// it, the icons are the config's default.
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
