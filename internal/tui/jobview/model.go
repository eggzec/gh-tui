// Package jobview shows one job of a GitHub Actions run: its log, with the
// failed step open on its first error, or its steps while GitHub doesn't
// publish the log yet. The Actions modal shows the
// job under the cursor of its jobs with it, and the checks of a pull
// request the job of a check.
//
// A job's log is published whole only when the job ends, so the view of a
// job in progress shows its steps as they run instead, or the part of its
// log that GitHub already publishes, which grows as the parent's poll of
// the run reads more, and the whole log once the parent shows the job
// again, done.
package jobview

import (
	"context"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// Service is what the view reads a job's log from.
type Service interface {
	// CachedLog returns a job's log from memory, without a request.
	CachedLog(repo core.RepoRef, jobID int64) (core.Log, bool)
	Log(ctx context.Context, repo core.RepoRef, jobID int64) (core.Log, error)
	// CachedPartialLog returns what was read of the log of a job in
	// progress from memory, without a request.
	CachedPartialLog(repo core.RepoRef, jobID int64) (core.PartialLog, bool)
	PartialLog(ctx context.Context, repo core.RepoRef, jobID int64) (core.PartialLog, error)
	// WatchLog has the poll of the run of a job in progress read what is
	// added to its log, until stop is called.
	WatchLog(repo core.RepoRef, runID, jobID int64) (stop func())
}

// KeyMap holds the keys of the view. The parent handles its own keys
// first, so these leave out any it takes.
type KeyMap struct {
	// Log moves through the log.
	Log logview.KeyMap
	// LogContext names the context of keys that the log is, in the parent's
	// modal, for help.
	LogContext string
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding { return nil }

// FullHelp implements help.KeyMap: the keys of the log.
func (k KeyMap) FullHelp() [][]key.Binding { return k.Log.FullHelp() }

// State is what the view shows.
type State int

const (
	// None shows no job.
	None State = iota
	// Pending shows the steps of a job that hasn't finished, whose log
	// GitHub doesn't publish yet.
	Pending
	// Loading waits for the log.
	Loading
	// Ready shows the log.
	Ready
	// Partial shows the log of a job in progress as far as GitHub
	// publishes it, which grows as the poll of its run reads more.
	Partial
	// Expired shows the steps of a job whose log GitHub no longer keeps.
	Expired
	// Failed shows why the log failed to load.
	Failed
)

var lastID atomic.Int64

// Model shows a job. Create it with [New]. It starts blurred.
type Model struct {
	id   int64
	ctx  context.Context
	svc  Service
	repo core.RepoRef
	opts options
	keys KeyMap

	view  logview.Model
	job   core.Job
	state State
	// failed is why the log failed to load, while the state is Failed.
	failed error
	// focused is set while the view takes keys.
	focused bool
	// truncated reports that only the end of the log was read.
	truncated bool
	// resting is set while the log waits for the cursor of the parent to
	// rest on the job before it is read, and seq counts the rests.
	resting bool
	seq     int
	// part is what the partial log shows, and unwatch stops the reads of
	// it while the job runs.
	part    shownPart
	unwatch func()
	// find is what the log opens on, from [Hints].
	find []string
	// pos is where the log goes once it shows, set by SetPosition while
	// it loads.
	pos *pager.Position

	width, height int
	st            ui.RunStyles
	errs          ui.ErrorStyles
}

// Hints say where a log opens, when the caller knows more than the log.
type Hints struct {
	// Find lists texts, best first: the log opens on the first line that
	// contains the first of them that any line contains, and so shows the
	// step that printed it. When none appears, the log opens as it does
	// without hints, at the failed step.
	Find []string
}

// Option configures a Model in [New].
type Option func(*options)

type options struct {
	icons ui.Icons
	rest  time.Duration
	now   func() time.Time
	voice ui.Voice
}

// WithVoice sets how the view words a log or annotations that failed to
// load, with the
// keys a hint names and the log file it points to. By default it names no
// keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(o *options) { o.voice = v }
}

// WithIcons sets the glyphs of the states of the steps. Without it, the
// icons are the config's default.
func WithIcons(ic ui.Icons) Option {
	return func(o *options) { o.icons = ic }
}

// WithRest sets how long a job shown with rest set waits before its log is
// read, so that a scroll through the jobs doesn't read each. Zero reads at
// once.
func WithRest(d time.Duration) Option {
	return func(o *options) { o.rest = d }
}

// WithClock sets the clock that the times of steps in progress count to.
// The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(o *options) { o.now = now }
}

// New returns a view that shows no job, which reads the logs of repo from
// svc under ctx.
func New(ctx context.Context, svc Service, repo core.RepoRef, keys KeyMap, opts ...Option) Model {
	o := options{icons: ui.NewIcons(config.Default().UI.Icons), now: time.Now}
	for _, opt := range opts {
		opt(&o)
	}
	o.voice.Icons = &o.icons
	m := Model{
		id:   lastID.Add(1),
		ctx:  ctx,
		svc:  svc,
		repo: repo,
		opts: o,
		keys: keys,
		view: logview.New(logview.WithFocusFailed(true), logview.WithKeyMap(keys.Log),
			logview.WithErrorText(ui.ErrorText("load the log", repo.String(), o.voice))),
	}
	return m
}

// SetIcons takes the icon set ic, which the theme set after draws with. The
// voice keeps pointing to the copy that New made, which the log's error text
// reads, so that is the one changed too.
func (m *Model) SetIcons(ic ui.Icons) {
	m.opts.icons = ic
	*m.opts.voice.Icons = ic
}

// SetTheme styles the view.
func (m *Model) SetTheme(t ui.Theme) {
	m.st = ui.NewRunStyles(t, m.opts.icons)
	m.errs = t.Errors(m.opts.icons)
	m.view.SetStyles(t.LogView(m.opts.icons))
}

// SetSize sets the size of the view.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.layout()
}

// layout sizes the log to the room below the notice.
func (m *Model) layout() {
	h := m.height
	if m.notice() != "" {
		h--
	}
	m.view.SetSize(m.width, max(h, 0))
}

// Focus makes the view take keys.
func (m *Model) Focus() {
	m.focused = true
	m.view.Focus()
}

// Blur makes the view ignore keys.
func (m *Model) Blur() {
	m.focused = false
	m.view.Blur()
}

// Focused reports whether the view takes keys.
func (m Model) Focused() bool { return m.focused }

// Capturing reports whether the search of the log takes every key.
func (m Model) Capturing() bool { return m.view.Capturing() }

// Query is the search of the log, or empty.
func (m Model) Query() string { return m.view.Query() }

// Lines counts the lines of the log.
func (m Model) Lines() int { return m.view.Lines() }

// Errors counts the errors of the log.
func (m Model) Errors() int { return m.view.Errors() }

// LogID is the ID of the log view, whose logview.CloseMsg asks the parent
// to step back.
func (m Model) LogID() int64 { return m.view.ID() }

// KeyMap returns the keys of the log.
func (m Model) KeyMap() logview.KeyMap { return m.view.KeyMap() }

// KeyLayers returns the keys of the view as a layer of keys.
func (m Model) KeyLayers() []keyhelp.Layer { return []keyhelp.Layer{m.Layer()} }

// Layer returns the keys of the view in the order it matches them: its own,
// and then those of the log, once it shows. It is a layer of the context of
// the log, or of a search of the log while that takes every key.
func (m Model) Layer() keyhelp.Layer {
	ctx := m.keys.LogContext
	log := keyhelp.FromHelp("", m.view, m.view.Searching())
	limit := func(bs []key.Binding) {
		for i, b := range bs {
			// The log's keys do nothing until it shows.
			bs[i].SetEnabled(b.Enabled() && m.showsLog())
		}
	}
	limit(log.Bindings)
	limit(log.Short)
	switch {
	case m.view.Searching():
		// A search of the log takes every key.
		ctx = "search_prompt"
	case m.view.ChoosingOption():
		// So does the name of an option, but it types nothing.
		ctx = "log_option"
	}
	return ui.MergeLayers(ctx, log)
}

// State is what the view shows.
func (m Model) State() State { return m.state }

// showsLog reports whether the view shows a log, whole or partial, which
// the keys of the log move through.
func (m Model) showsLog() bool { return m.state == Ready || m.state == Partial }

// Job returns the job shown, if any.
func (m Model) Job() (core.Job, bool) {
	return m.job, m.state != None
}

// JobID is the ID of the job shown, or 0.
func (m Model) JobID() int64 {
	if m.state == None {
		return 0
	}
	return m.job.ID
}

// Title names the job shown, or is empty.
func (m Model) Title() string {
	if m.state == None {
		return ""
	}
	return ui.OneLine(m.job.Name)
}

// Update takes the view's rests and log, and passes the rest to the log.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case restMsg:
		if msg.id != m.id || msg.seq != m.seq || !m.resting {
			return m, nil
		}
		m.resting = false
		cmd := m.read()
		return m, cmd
	case logMsg:
		if msg.id == m.id {
			m.receive(msg)
		}
		return m, nil
	case partialMsg:
		if msg.id == m.id {
			m.receivePartial(msg)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.view, cmd = m.view.Update(msg)
	return m, cmd
}
