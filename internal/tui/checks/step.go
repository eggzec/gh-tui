// Package checks is the Checks step of the modal of a pull request: the
// checks of its head commit, grouped by workflow, with the commit statuses
// after them and a count of those failing, pending and passed at the top.
// It opens on the first failing check.
//
// A check that reports a job of GitHub Actions opens into the job's log in
// the same frame, with the annotations of a failed job above it, and
// follows the job while it runs; the failed jobs of its run can be re-run.
// Any other check shows the title, summary and text its app reported, and a
// commit status its description. While a check is pending, the sync engine
// polls the checks, until they are all done, the step is hidden, or it
// closes.
package checks

import (
	"context"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Title names the step, and its changes in the log and the toasts.
const Title = "Checks"

// Service is what the step needs from the actions service.
type Service interface {
	jobview.Service
	// CachedChecks returns the checks of a commit from memory, without a
	// request.
	CachedChecks(q actionssvc.ChecksQuery) (core.Checks, bool)
	Checks(ctx context.Context, q actionssvc.ChecksQuery) (core.Checks, error)
	// CachedRun returns a run from memory, without a request, as a change
	// or a poll left it.
	CachedRun(repo core.RepoRef, runID int64) (core.Run, bool)
	Run(ctx context.Context, repo core.RepoRef, runID int64) (core.Run, error)
	// CachedAllJobs returns the jobs of an attempt from memory, without a
	// request.
	CachedAllJobs(q actionssvc.JobsQuery) (core.Page[core.Job], bool)
	// AllJobs returns the jobs of an attempt, every page of them up to
	// actionssvc.MaxJobPages.
	AllJobs(ctx context.Context, q actionssvc.JobsQuery) (core.Page[core.Job], error)
	// Invalidate marks what is cached of repo stale, so that the reads
	// after it ask GitHub.
	Invalidate(repo core.RepoRef)
	// RerunFailedJobs shows the failed jobs of a run queued in the cache
	// at once. The returned Op sends the re-run.
	RerunFailedJobs(repo core.RepoRef, runID int64) *optimistic.Op
}

// Watch starts polling the checks of q, so that their changes arrive as
// ui.SyncMsg under actionssvc.ChecksSyncKey(q), and returns the function
// that stops it. It must not block.
type Watch func(q actionssvc.ChecksQuery) (stop func())

// Follow starts polling run runID of repo while it runs, so that its
// changes arrive as ui.SyncMsg under actionssvc.RunSyncKey, and returns the
// function that stops it. It must not block.
type Follow func(repo core.RepoRef, runID int64) (stop func())

// Option configures a Step in [New].
type Option func(*options)

type options struct {
	icons  ui.Icons
	watch  Watch
	follow Follow
	ret    ui.Modal
	caps   core.RepoCaps
	// voice words a log that failed to load, or is nil for one of the
	// keys New is given.
	voice *ui.Voice
	// tick is how often the timers of what runs move on; tests set 0,
	// which stops them.
	tick time.Duration
	now  func() time.Time
}

// WithCaps sets what the viewer may do in the repository, as far as it is
// known, which decides whether a re-run is offered. A ui.CapsMsg updates
// it. The default is unknown, which offers it.
func WithCaps(c core.RepoCaps) Option {
	return func(o *options) { o.caps = c }
}

// WithVoice sets how the step words what went wrong, with the keys a hint
// names and the log it points to. By default the hints name the configured
// keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(o *options) { o.voice = &v }
}

// WithIcons sets the glyphs of the states of the checks. Without it, the
// icons are the config's default.
func WithIcons(ic ui.Icons) Option {
	return func(o *options) { o.icons = ic }
}

// WithWatch sets how the step polls the checks while some are pending.
// Without it, they change only when they are read again.
func WithWatch(w Watch) Option {
	return func(o *options) { o.watch = w }
}

// WithFollow sets how the step follows the run of a job in progress.
// Without it, the job changes only when it is read again.
func WithFollow(f Follow) Option {
	return func(o *options) { o.follow = f }
}

// WithReturn sets the modal the step is shown in, which a file preview
// opened from an annotation reopens when it closes.
func WithReturn(m ui.Modal) Option {
	return func(o *options) { o.ret = m }
}

// WithTick sets how often the timers of what runs move on. Zero stops
// them, which a parent's tests need, as their time doesn't pass; it is
// exported for them, since they live in other packages. The default is a
// second.
func WithTick(d time.Duration) Option {
	return func(o *options) { o.tick = d }
}

// WithClock sets the clock that the times of checks count to. The default
// is time.Now.
func WithClock(now func() time.Time) Option {
	return func(o *options) { o.now = now }
}

// mode is what the step shows.
type mode int

const (
	// listMode shows the checks.
	listMode mode = iota
	// jobMode shows the job of a check of GitHub Actions.
	jobMode
	// detailMode shows what the app of any other check reported, or a
	// commit status.
	detailMode
)

var lastID atomic.Int64

// Step is the Checks step. Create one with [New], and Init it once it is
// shown.
type Step struct {
	id int64
	// ctx bounds the step's reads, and cancel ends them when it closes.
	// parent bounds its changes, which go on after it closes.
	ctx    context.Context
	cancel context.CancelFunc
	parent context.Context
	svc    Service
	q      actionssvc.ChecksQuery
	keys   KeyMap
	opts   options

	checks core.Checks
	// loaded is set once the checks arrived, loading while they are read,
	// and err once a read failed.
	loaded, loading bool
	err             error
	rows            []row
	cursor, top     int
	// opened is set once the cursor went to the first failing check, which
	// it does once.
	opened bool

	mode mode
	// check is the check a job or a detail shows.
	check row
	job   jobState
	view  jobview.Model

	detail viewport.Model
	// md renders the detail, and rendered names the check and the width
	// it last rendered it for.
	md       *markdown.Renderer
	rendered string

	// stopWatch ends the polls of the checks, and stopFollow those of the
	// run of the job shown. hidden is set while a preview hides the step.
	stopWatch, stopFollow func()
	following             int64
	hidden                bool
	ticking               bool

	ask    *ui.Confirm
	notice string

	spin     spinner.Model
	spinning bool

	// voice words why the checks or the job failed to load.
	voice ui.Voice

	width, height int
	theme         ui.Theme
	st            styles
	errs          ui.ErrorStyles
	// links keeps the links of the rows, which are drawn on every frame.
	links termtext.Links
}

// New returns the Checks step of pull request number of repo, with the
// configured keys. ctx bounds its reads until it closes, and its changes.
func New(ctx context.Context, svc Service, repo core.RepoRef, number int, keys config.Keymap, opts ...Option) *Step {
	o := options{icons: ui.NewIcons(config.Default().UI.Icons), tick: time.Second, now: time.Now}
	for _, opt := range opts {
		opt(&o)
	}
	rctx, cancel := context.WithCancel(ctx)
	s := &Step{
		id:     lastID.Add(1),
		ctx:    rctx,
		cancel: cancel,
		parent: ctx,
		svc:    svc,
		q:      actionssvc.ChecksQuery{Repo: repo, Number: number},
		keys:   newKeyMap(keys),
		opts:   o,
		spin:   spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	// What failed is read again with the step's refresh key, which leaves
	// the key that re-runs.
	v := ui.NewVoice(keys, "")
	if o.voice != nil {
		v = *o.voice
	}
	v.Retry, v.Icons = s.keys.Refresh, &s.opts.icons
	s.voice = v
	s.view = jobview.New(rctx, svc, repo, s.keys.job(),
		jobview.WithIcons(o.icons), jobview.WithClock(o.now), jobview.WithReturn(o.ret), jobview.WithVoice(v))
	s.detail = viewport.New()
	s.detail.KeyMap = s.keys.Detail
	// Assume a dark terminal until the parent sets the theme.
	p, _ := config.Default().Palette(true)
	s.SetTheme(ui.NewTheme(p, true))
	if c, ok := svc.CachedChecks(s.q); ok {
		s.setChecks(c)
	}
	return s
}

// Init reads the checks, which show from memory meanwhile if they are
// there, and a fresh copy costs no request.
func (s *Step) Init() tea.Cmd {
	s.syncWatch()
	return tea.Batch(s.read(), s.startTick())
}

// Checks returns the checks shown, once they arrived.
func (s *Step) Checks() (core.Checks, bool) {
	return s.checks, s.loaded
}

// SetSize sizes the step to the room inside the frame.
func (s *Step) SetSize(width, height int) {
	s.width, s.height = max(width, 0), max(height, 0)
	s.layout()
}

// SetTheme styles the step and the bubbles in it.
func (s *Step) SetTheme(t ui.Theme) {
	s.voice.Icons = &s.opts.icons
	s.theme = t
	s.st = newStyles(t, s.opts.icons)
	s.errs = t.Errors(s.opts.icons)
	s.spin.Style = t.Accent
	s.spin.Spinner = s.opts.icons.SpinnerOr(spinner.Dot)
	s.view.SetTheme(t)
	s.md, s.rendered = nil, ""
	s.layout()
}

// Close ends the step's reads and polls. The parent calls it when it
// leaves the step or closes.
func (s *Step) Close() {
	s.unwatch()
	s.unfollow()
	s.view.Clear()
	s.cancel()
}

// now reads the clock.
func (s *Step) now() time.Time {
	return s.opts.now()
}

// startSpinner starts the spinner while something loads, unless it runs.
func (s *Step) startSpinner() tea.Cmd {
	if s.spinning {
		return nil
	}
	s.spinning = true
	return s.spin.Tick
}

// loadingAny reports whether the step waits for something the spinner
// shows.
func (s *Step) loadingAny() bool {
	return s.loading && !s.loaded || s.mode == jobMode && s.job.loading
}

// CloseMsg asks the parent to close the modal the step with ID is in, which
// the dismiss key sends once there is nothing in the step left to dismiss.
type CloseMsg struct {
	ID int64
}

// ID returns the instance ID that scopes the step's messages.
func (s *Step) ID() int64 { return s.id }
