// Package actions is the Actions modal of the repository screen: the
// workflow runs of the repository, newest first, the jobs of the run under
// the cursor, and the log of the job under theirs, in three panes of one
// frame. Tabs narrow the runs to those failing, running or started by the
// user, and a filter step narrows them further. The run shown can be
// re-run or cancelled, once the user confirms.
//
// The jobs follow the cursor of the runs, and the log that of the jobs,
// once it rests. A job's log is published only when the job ends, so the
// log pane of a job in progress shows its steps as they run instead,
// while the sync engine follows the run; the log loads once the job ends.
//
// On a narrow terminal the modal shows one pane at a time, with a
// breadcrumb of where it is.
package actions

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Title names the modal in the frame, and the changes it sends in the log.
const Title = "Actions"

// narrowWidth is the width inside the frame below which the panes don't fit
// side by side, so the modal shows one at a time.
const narrowWidth = 110

// pane is one of the modal's panes, in the order the focus moves through
// them.
type pane int

const (
	runsPane pane = iota
	jobsPane
	logPane
)

// numPanes is how many panes there are.
const numPanes = 3

var lastID atomic.Int64

// Modal is the Actions modal. Create one with [New].
type Modal struct {
	id int64
	// ctx bounds the modal's reads, and cancel ends them when it closes.
	// parent bounds the changes, which go on after it closes.
	ctx    context.Context
	cancel context.CancelFunc
	parent context.Context
	svc    Service
	repo   core.RepoRef
	keys   KeyMap
	opts   options
	// caps is what the viewer may do in repo, as far as it is known.
	caps core.RepoCaps

	focus pane
	zoom  bool

	// filter selects the runs; the tab shown is the one it matches.
	filter core.RunFilter
	runs   feed.Model[core.Run]
	// live holds the runs as a change or a poll left them in the cache,
	// newer than the feed's copies.
	live map[int64]core.Run

	// run is the run under the cursor of the runs, once there is one.
	run    core.Run
	hasRun bool
	jobs   jobs
	log    jobview.Model
	// seq counts the moves of the cursor of the runs, so that only the
	// last rest reads jobs.
	seq int

	// filterStep is open while the filter step is shown.
	filterStep *filterStep
	workflows  workflows

	// ask is the confirmation on the last line, and notice a line that
	// tells why a key did nothing, until the next key.
	ask    *ui.Confirm
	notice string

	// following is the run the sync engine follows, and stop ends it.
	following int64
	stop      func()
	ticking   bool

	spin     spinner.Model
	spinning bool

	width, height int
	theme         ui.Theme
	st            styles
	// links keeps the links of the rows, which are drawn on every frame.
	links termtext.Links
}

var _ ui.Modal = (*Modal)(nil)

// Opener returns what the app opens the Actions modal with, for
// tui.WithActions: a new modal each time, which reads what it shows from
// svc and takes its keys from the configured keys.
func Opener(svc Service, keys map[string][]string, opts ...Option) func(ctx context.Context, repo core.RepoRef, f core.RunFilter) (ui.Modal, tea.Cmd) {
	return func(ctx context.Context, repo core.RepoRef, f core.RunFilter) (ui.Modal, tea.Cmd) {
		m := New(ctx, svc, repo, keys, append(slices.Clip(opts), WithFilter(f))...)
		load := m.Init()
		return m, load
	}
}

// New returns the Actions modal of repo, which shows every run, with the
// runs focused. ctx bounds its reads until it closes, and its changes.
// Call Init once it is open.
func New(ctx context.Context, svc Service, repo core.RepoRef, keys map[string][]string, opts ...Option) *Modal {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	rctx, cancel := context.WithCancel(ctx)
	keyMap := newKeyMap(keys)
	m := &Modal{
		id:     lastID.Add(1),
		ctx:    rctx,
		cancel: cancel,
		parent: ctx,
		svc:    svc,
		repo:   repo,
		keys:   keyMap,
		opts:   o,
		live:   map[int64]core.Run{},
		spin:   spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	// Assume a dark terminal until the app sets the theme.
	p, _ := config.Default().Palette(true)
	m.log = jobview.New(rctx, svc, repo, keyMap.job(),
		jobview.WithIcons(o.icons), jobview.WithRest(o.rest), jobview.WithClock(o.now), jobview.WithReturn(m))
	m.SetTheme(ui.NewTheme(p, true))
	m.filter = o.filter
	m.caps = ui.CachedCaps(o.repos, repo)
	m.runs = m.newRuns()
	m.setFocus(runsPane)
	return m
}

// Init reads the first runs, and what the viewer may do in the repository
// unless it is known.
func (m *Modal) Init() tea.Cmd {
	if m.caps.Known {
		return m.runs.Init()
	}
	return tea.Batch(m.runs.Init(), ui.LoadCaps(m.ctx, m.opts.repos, m.repo))
}

// Title names the repository.
func (m *Modal) Title() string {
	return Title + " · " + m.repo.String()
}

// Tabs returns the names of the tabs, which the app shows in the top edge
// of the frame, and the one shown, or -1 when the filter matches none.
func (m *Modal) Tabs() (names []string, active int) {
	n := m.numTabs()
	t, ok := tabOf(m.filter)
	if !ok || int(t) >= n {
		return tabNames[:n], -1
	}
	return tabNames[:n], int(t)
}

// SetSize sizes the panes to the room inside the frame.
func (m *Modal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.layout()
}

// SetTheme styles the modal and the bubbles in it.
func (m *Modal) SetTheme(t ui.Theme) {
	m.theme = t
	m.st = newStyles(t, m.opts.icons)
	m.spin.Style = t.Accent
	m.runs.SetStyles(t.Feed())
	m.log.SetTheme(t)
	if f := m.filterStep; f != nil && f.form != nil {
		f.form.SetStyles(t.FilterForm())
	}
}

// Help lists the keys of the focused pane.
func (m *Modal) Help() help.KeyMap {
	return helpKeys{m: m}
}

// narrow reports whether the modal shows one pane at a time.
func (m *Modal) narrow() bool {
	return m.width < narrowWidth
}

// close ends the modal's reads and the following of its run, and asks the
// app to close it.
func (m *Modal) close() tea.Cmd {
	m.unfollow()
	m.cancel()
	return ui.CloseModal(m)
}

// startSpinner starts the spinner while something loads, unless it runs.
func (m *Modal) startSpinner() tea.Cmd {
	if m.spinning {
		return nil
	}
	m.spinning = true
	return m.spin.Tick
}

// loading reports whether a pane waits for something the spinner shows.
func (m *Modal) loading() bool {
	return m.jobs.loading || m.workflows.loading
}

// now reads the clock.
func (m *Modal) now() time.Time {
	return m.opts.now()
}

// rest returns the message that a rest of the cursor of the runs sends,
// after the delay the options set.
func (m *Modal) rest() tea.Cmd {
	m.seq++
	msg := restMsg{id: m.id, seq: m.seq}
	if m.opts.rest <= 0 {
		return func() tea.Msg { return msg }
	}
	return tea.Tick(m.opts.rest, func(time.Time) tea.Msg { return msg })
}

// restMsg reports that the cursor of the runs rested, for seq.
type restMsg struct {
	id  int64
	seq int
}
