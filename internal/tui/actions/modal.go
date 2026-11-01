// Package actions is the Actions modal of the repository screen: the
// workflow runs of the repository, newest first, and the jobs of the run
// under the cursor, in two panes of one frame. Tabs narrow the runs to those failing, running or started by the
// user, and a filter step narrows them further. The run shown can be
// re-run or cancelled, once the user confirms.
//
// The jobs follow the cursor of the runs. Enter on a job opens its log in an
// overlay over the modal (logModal). The jobs of the runs around the cursor,
// and the logs of the failed jobs around that of the jobs, are read ahead as
// prefetch.actions says, so that they show at once. A job's log is
// published whole only when the job ends, so the overlay of a job in
// progress shows its steps as they run instead, or the part of its log
// that GitHub already publishes, while the sync engine follows the run;
// the whole log loads once the job ends.
package actions

import (
	"context"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Title names the modal in the frame, and the changes it sends in the log.
const Title = "Actions"

// pane is one of the modal's panes, in the order the focus moves through
// them.
type pane int

const (
	runsPane pane = iota
)

// numPanes is how many boxes the focus moves through: the runs, with their
// jobs under them, and the annotations.
const numPanes = 2

// notesPane is the stop of the annotations. It isn't a pane of the runs
// layout, so the modal tracks it with notes, not focus.
const notesPane pane = numPanes - 1

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
	// notes is set while the annotations box has the focus, and zoom while
	// the focused box fills the modal.
	notes, zoom bool
	// frames draws the boxes. inner is the width inside the runs box, and
	// rows the height of the runs and the jobs under their titles.
	frames panes.Styles
	inner  int
	rows   [runsPane + 1]int

	// filter selects the runs; the tab shown is the one it matches.
	filter core.RunFilter
	// toggle holds the filter that the filter toggle key switched off.
	toggle ui.FilterToggle[core.RunFilter]
	runs   feed.Model[core.Run]
	// live holds the runs as a change or a poll left them in the cache,
	// newer than the feed's copies.
	live map[int64]core.Run
	// levels are the fields the runs are grouped by, off while the grouping
	// is switched off; sorts and sortOff are the same for the sort.
	levels, sorts []orderpick.Choice
	off, sortOff  bool

	// run is the run under the cursor of the runs, once there is one.
	run    core.Run
	hasRun bool
	// jobsOf holds the jobs read for each run, by run ID.
	jobsOf map[int64]*jobs
	// ann holds the annotations read for the jobs of the run shown, by job,
	// and noteCursor and noteTop the cursor and the first row of their list.
	ann                 map[int64]*noteRes
	noteCursor, noteTop int
	// wantJob is the job the first jobs shown open on, until they are.
	wantJob int64
	// seq counts the moves of the cursor of the runs, so that only the
	// last rest reads jobs.
	seq int
	// aheadJobs reads the jobs of the runs around the cursor of the runs,
	// and aheadLogs the logs of the failed jobs around that of the jobs.
	aheadJobs *ui.Ahead[actionssvc.JobsQuery]
	aheadLogs *ui.Ahead[int64]

	workflows workflows

	// ask is the confirmation on the last line, and notice a line that
	// tells why a key did nothing, until the next key.
	ask *confirm.Ask
	// armed is set once the question is armed: until then enter answers
	// nothing.
	armed  bool
	notice string

	// follows holds, by run ID, what ends the sync engine's following of each
	// run kept up to date: the run shown while it runs, and the open runs
	// whose jobs are loaded while they run.
	follows map[int64]func()
	ticking bool
	// at is the run and the row of its jobs that the cursor was on when the
	// job shown was last chosen.
	at shownAt

	spin ui.Spinner

	width, height int
	theme         ui.Theme
	st            styles
	errs          ui.ErrorStyles
	// links keeps the links of the rows, which are drawn on every frame.
	links termtext.Links
}

var _ ui.Modal = (*Modal)(nil)

// Opener returns what the app opens the Actions modal with, for
// tui.WithActions: a new modal each time, which reads what it shows from
// svc and takes its keys from the configured keys. It opens on the run, the
// job and the pane that its last argument names, if it names any.
func Opener(svc Service, keys config.Keymap, opts ...Option) func(ctx context.Context, repo core.RepoRef, f core.RunFilter, at ui.ActionsAt) (ui.Modal, tea.Cmd) {
	return func(ctx context.Context, repo core.RepoRef, f core.RunFilter, at ui.ActionsAt) (ui.Modal, tea.Cmd) {
		opts := append(slices.Clip(opts), WithFilter(f), WithRun(at.Run), WithJob(at.Job))
		m := New(ctx, svc, repo, keys, opts...)
		load := m.Init()
		return m, load
	}
}

// New returns the Actions modal of repo, which shows every run, with the
// runs focused. ctx bounds its reads until it closes, and its changes.
// Call Init once it is open.
func New(ctx context.Context, svc Service, repo core.RepoRef, keys config.Keymap, opts ...Option) *Modal {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	rctx, cancel := context.WithCancel(ctx)
	keyMap := newKeyMap(keys)
	// What failed is read again with the modal's refresh key.
	v := ui.NewVoice(keys, "")
	if o.voice != nil {
		v = *o.voice
	}
	v.Retry, v.Icons = keyMap.Refresh, &o.icons
	o.voice = &v
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
		levels: defaultGroups(),
		spin:   ui.NewSpinner(spinner.Dot),
	}
	// Assume a dark terminal until the app sets the theme.
	p, _ := config.Default().Palette(true)
	m.aheadJobs = ui.NewAhead("jobs", m.readJobsAhead, m.cachedJobs)
	m.aheadJobs.Share(o.slots)
	m.aheadJobs.Reset(rctx)
	m.aheadJobs.Configure(o.prefetch.jobs)
	m.aheadLogs = ui.NewAhead("job_log", m.readLogAhead, m.cachedLog)
	m.aheadLogs.Share(o.slots)
	m.aheadLogs.Reset(rctx)
	m.aheadLogs.Configure(o.prefetch.logs)
	m.SetTheme(ui.NewTheme(p, true))
	m.filter = o.filter
	m.caps = ui.CachedCaps(o.repos, repo)
	m.runs = m.newRuns()
	if o.run != 0 {
		m.runs.Seek(strconv.FormatInt(o.run, 10))
	}
	m.wantJob = o.job
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
	return ui.Title(ui.TitleSpec{Name: Title, Chips: []string{m.repo.String()}, Sep: m.opts.icons.Separator})
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
	m.opts.voice.Icons = &m.opts.icons
	m.theme = t
	m.st = newStyles(t, m.opts.icons)
	m.errs = t.Errors(m.opts.icons)
	m.frames = panes.DefaultStyles(t.Dark)
	m.frames.Border = m.opts.icons.Border
	m.frames.Ellipsis = m.opts.icons.Ellipsis
	m.frames.Separator = m.opts.icons.Separator
	m.spin.Style = t.Accent
	m.spin.Spinner = m.opts.icons.SpinnerOr(spinner.Dot)
	m.runs.SetStyles(t.Feed(m.opts.icons))
}

// Commands implements ui.Commanded: the run, or the job, can be copied.
func (m *Modal) Commands() []string { return []string{ui.CommandCopy} }

// Selected implements ui.Selector, for the copy command: the job that the
// open key would open, in the jobs and the log, or else the run, with the
// commit the run ran on.
func (m *Modal) Selected() (ui.Selection, bool) {
	if !m.hasRun {
		return ui.Selection{}, false
	}
	sel := ui.Selection{What: "run", URL: m.run.URL, Repo: m.repo, SHA: m.run.HeadSHA, Owner: m.repo.Owner}
	if m.onJobRow() {
		if j, ok := m.cursorJob(); ok {
			sel.What, sel.URL = "job", j.URL
		}
	}
	return sel, true
}

// KeyLayers implements ui.Keyed. The confirmation, the filter and a search
// of the log each take every key while open; otherwise the modal's own
// keys come first, named for what they do in the focused pane, and then
// those of the pane.
func (m *Modal) KeyLayers() []keyhelp.Layer {
	k := m.keys
	switch {
	case m.ask != nil:
		return []keyhelp.Layer{ui.ConfirmLayer(k.Confirm)}
	case m.focus == runsPane && m.runs.Capturing():
		// The prompt of the runs' find or filter takes every key.
		list, _ := ui.FeedLayer(m.runs)
		return []keyhelp.Layer{list}
	}
	k = k.state(m)
	screen := ui.ContextLayer(ctxModal, k.screen(), []key.Binding{k.Next, k.Cancel, k.RerunFailed, k.Rerun, k.Open})
	if m.notes {
		return []keyhelp.Layer{screen}
	}
	runs := keyhelp.Layer{Bindings: []key.Binding{k.Select, k.Filter, k.ClearFilter, k.ToggleFilter, k.RerunJob}, Short: []key.Binding{k.Select, k.RerunJob, k.Filter}}
	list, _ := ui.FeedLayer(m.runs)
	merged := ui.MergeLayers(ctxRuns, runs, list)
	// The action of the row comes before the moves of the list: a narrow
	// status line sheds hints from its end, and the moves are the ones to go.
	merged.Short = slices.Concat([]key.Binding{k.Select, k.RerunJob}, list.Short, []key.Binding{k.Filter})
	return []keyhelp.Layer{screen, merged}
}

// selectLabel names what select does on the row under the cursor: it opens
// the log of a job, and folds or unfolds a run or a group of jobs.
func (m *Modal) selectLabel() string {
	switch {
	case m.onJobRow():
		return "log"
	case m.onGroupRow():
		j := m.cur()
		k, _ := m.childOf(j)
		return foldLabel(j.isOpen(j.lines[k].group.key))
	}
	return foldLabel(m.runs.Unfolded())
}

// foldLabel is "fold" for what is open, else "unfold".
func foldLabel(open bool) string {
	if open {
		return "fold"
	}
	return "unfold"
}

// state returns k as the modal takes it now, named for what the keys do
// on the row under the cursor: select folds, unfolds or opens the log, back
// names the box before and is off on the runs, and a run offers the changes
// that apply to it, if the viewer may make them.
func (k KeyMap) state(m *Modal) KeyMap {
	// Enter folds a run or a group of jobs, and opens the log of a job.
	k.Select = relabel(k.Select, m.selectLabel())
	{
		// A filter that is switched off is kept, so the keys have it to act on.
		set := formFiltered(m.filter) || m.toggle.Off()
		k.ClearFilter.SetEnabled(k.ClearFilter.Enabled() && set)
		k.ToggleFilter.SetEnabled(k.ToggleFilter.Enabled() && set)
		// Nothing is before the runs.
		k.Back.SetEnabled(false)
		switch {
		case m.runs.FindQuery() != "":
			k.Dismiss = relabel(k.Dismiss, "clear find")
		case m.runs.FilterQuery() != "":
			k.Dismiss = relabel(k.Dismiss, "clear filter")
		}
	}
	if m.notes {
		// Back leaves the annotations for the runs.
		k.Back.SetEnabled(true)
		k.Back = relabel(k.Back, paneTitles[runsPane])
	}
	k.Zoom.SetEnabled(k.Zoom.Enabled() && m.canZoom())
	g := m.gate()
	done := m.hasRun && m.run.Done()
	k.Cancel.SetEnabled(k.Cancel.Enabled() && m.hasRun && !m.run.Done())
	k.RerunFailed.SetEnabled(k.RerunFailed.Enabled() && done)
	k.Rerun.SetEnabled(k.Rerun.Enabled() && done)
	k.RerunJob.SetEnabled(k.RerunJob.Enabled() && done && m.onJobRow())
	k.RerunJob = g.Gated(k.RerunJob, ui.ActRerun, nil)
	k.Cancel, k.RerunFailed = g.Gated(k.Cancel, ui.ActCancelRun, nil), g.Gated(k.RerunFailed, ui.ActRerun, nil)
	k.Rerun = g.Gated(k.Rerun, ui.ActRerun, nil)
	// A key that re-run shares with refresh says why it can't re-run
	// rather than refresh.
	k.Refresh = ui.Yield(k.Refresh, k.RerunFailed)
	k.Open.SetEnabled(k.Open.Enabled() && m.hasRun)
	k.ViewFile.SetEnabled(k.ViewFile.Enabled() && m.hasRun)
	// Only a question takes the answers.
	k.Confirm.Yes.SetEnabled(false)
	k.Confirm.No.SetEnabled(false)
	return k
}

// Hide implements ui.Hider: it stops following the run while another modal,
// such as the log of a job, is open in its place, until the modal is
// reopened.
func (m *Modal) Hide() {
	m.unfollow()
}

// Discard implements ui.Discarder: the modal ends for good, as when the
// log it opened closes the chain.
func (m *Modal) Discard() {
	m.end()
}

// close ends the modal's reads and the following of its run, and asks the
// app to close it.
func (m *Modal) close() tea.Cmd {
	m.end()
	return ui.CloseModal(m)
}

// leave ends the modal's reads as close does, and asks the app to return to
// the modal it was opened over, or to close it if there is none.
func (m *Modal) leave() tea.Cmd {
	m.end()
	return ui.LeaveModal(m)
}

// end stops the following of the run and the reads of the modal.
func (m *Modal) end() {
	m.unfollow()
	m.cancel()
}

// loading reports whether a pane waits for something the spinner shows.
func (m *Modal) loading() bool {
	return m.cur().loading || m.workflows.loading
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
	rest := m.opts.prefetch.jobs.Rest
	if rest <= 0 {
		return func() tea.Msg { return msg }
	}
	return tea.Tick(rest, func(time.Time) tea.Msg { return msg })
}

// restMsg reports that the cursor of the runs rested, for seq.
type restMsg struct {
	id  int64
	seq int
}
