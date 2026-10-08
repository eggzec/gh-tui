// Package jobview shows one job of a GitHub Actions run: its log, with the
// failed step open on its first error, or its steps while GitHub doesn't
// publish the log yet, and above them the annotations of a failed job,
// each of which opens its file on its line. The Actions modal shows the
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
	"slices"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
)

// Service is what the view reads a job's log and annotations from.
type Service interface {
	// CachedLog returns a job's log from memory, without a request.
	CachedLog(repo core.RepoRef, jobID int64) (core.Log, bool)
	Log(ctx context.Context, repo core.RepoRef, jobID int64) (core.Log, error)
	// CachedAnnotations returns a page of annotations from memory, without
	// a request.
	CachedAnnotations(q actionssvc.AnnotationsQuery) (core.Page[core.Annotation], bool)
	Annotations(ctx context.Context, q actionssvc.AnnotationsQuery) (core.Page[core.Annotation], error)
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
	// Annotations moves the focus from the log to the annotations, and
	// NotesAnnotations, the key the annotations have for it, back to the
	// log.
	Annotations      key.Binding
	NotesAnnotations key.Binding
	// LogContext and NotesContext name the contexts of keys that the log
	// and the annotations are, in the parent's modal, for help.
	LogContext, NotesContext string
	// Up and Down move through the annotations, and Select opens the file
	// of the one under the cursor.
	Up, Down, Select key.Binding
	// Open opens the job on GitHub, which the notices name.
	Open key.Binding
}

// own returns the keys of the view itself, in the order it matches them,
// with the key that moves the focus as the annotations or the log, whichever
// has it, has the key.
func (k KeyMap) own(onNotes bool) []key.Binding {
	return []key.Binding{k.annotations(onNotes), k.Up, k.Down, k.Select, k.Open}
}

// annotations returns the key that moves the focus to the other of the
// annotations and the log.
func (k KeyMap) annotations(onNotes bool) key.Binding {
	if onNotes {
		return k.NotesAnnotations
	}
	return k.Annotations
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding { return k.shortHelp(false) }

// FullHelp implements help.KeyMap: the keys of the view and the log.
func (k KeyMap) FullHelp() [][]key.Binding {
	return append([][]key.Binding{append(k.own(false), k.NotesAnnotations)}, k.Log.FullHelp()...)
}

// shortHelp returns the keys of the view worth a hint.
func (k KeyMap) shortHelp(onNotes bool) []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.annotations(onNotes)}
}

// Hints are what the parent knows of a job shown beyond the job itself.
type Hints struct {
	// SHA is the commit the job ran on, at which the files of its
	// annotations open.
	SHA string
	// NoAnnotations reports that the job is known to have none, such as
	// from its check run, so none are read.
	NoAnnotations bool
}

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
	hints Hints
	state State
	// failed is why the log failed to load, while the state is Failed.
	failed error
	notes  notes
	// focused is set while the view takes keys, and onNotes while the
	// annotations take them rather than the log.
	focused, onNotes bool
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

	width, height int
	st            ui.RunStyles
	errs          ui.ErrorStyles
}

// Option configures a Model in [New].
type Option func(*options)

type options struct {
	icons ui.Icons
	rest  time.Duration
	now   func() time.Time
	ret   ui.Modal
	voice ui.Voice
}

// WithVoice sets how the view words a log or annotations that failed to
// load, with the
// keys a hint names and the log file it points to. By default it names no
// keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(o *options) { o.voice = v }
}

// WithReturn sets the modal that a file preview opened from an annotation
// reopens when it closes: the one the view is shown in.
func WithReturn(m ui.Modal) Option {
	return func(o *options) { o.ret = m }
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

// SetTheme styles the view.
func (m *Model) SetTheme(t ui.Theme) {
	m.opts.voice.Icons = &m.opts.icons
	m.st = ui.NewRunStyles(t, m.opts.icons)
	m.errs = t.Errors(m.opts.icons)
	m.view.SetStyles(t.LogView(m.opts.icons))
}

// SetSize sets the size of the view.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.layout()
}

// layout sizes the log to the room below the notice and the annotations.
func (m *Model) layout() {
	h := m.height - m.notesHeight()
	if m.notice() != "" {
		h--
	}
	m.view.SetSize(m.width, max(h, 0))
	m.notes.scroll(m.noteRows())
}

// Focus makes the view take keys: the log, or the annotations if they had
// the focus.
func (m *Model) Focus() {
	m.focused = true
	m.focusLog(!m.onNotes || len(m.notes.items) == 0)
}

// Blur makes the view ignore keys.
func (m *Model) Blur() {
	m.focused = false
	m.view.Blur()
}

// focusLog gives the keys to the log, or to the annotations.
func (m *Model) focusLog(log bool) {
	m.onNotes = !log
	if log && m.focused {
		m.view.Focus()
	} else {
		m.view.Blur()
	}
}

// Focused reports whether the view takes keys.
func (m Model) Focused() bool { return m.focused }

// OnAnnotations reports whether the annotations take the keys rather than
// the log.
func (m Model) OnAnnotations() bool { return m.focused && m.onNotes }

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

// KeyLayers returns the keys of the view as a layer of keys, whichever of
// the log and the annotations has the focus.
func (m Model) KeyLayers() []keyhelp.Layer { return []keyhelp.Layer{m.Layer()} }

// Layer returns the keys of the view in the order it matches them: those
// of the annotations, which take every key but those of the whole log
// while they have the focus, and then those of the log, once it shows. It
// is a layer of the context of whichever has the focus, or of a search of
// the log while that takes every key.
func (m Model) Layer() keyhelp.Layer {
	k := m.keys.state(m)
	on := m.OnAnnotations()
	ctx := k.LogContext
	if on {
		ctx = k.NotesContext
	}
	notes := keyhelp.Layer{Bindings: k.own(m.OnAnnotations()), Short: k.shortHelp(on)}
	log := keyhelp.FromHelp("", m.view, m.view.Searching())
	whole := m.wholeLog()
	limit := func(bs []key.Binding) {
		for i, b := range bs {
			// The log's keys do nothing until it shows, and only those of the
			// whole log reach it from the annotations.
			keep := m.showsLog() && (!on || m.view.Capturing() || slices.ContainsFunc(whole, func(w key.Binding) bool {
				return slices.Equal(w.Keys(), b.Keys())
			}))
			bs[i].SetEnabled(b.Enabled() && keep)
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
	return ui.MergeLayers(ctx, notes, log)
}

// state returns k as the view takes it now: the annotations only while
// there are some, the keys that move through them while they have the
// focus, and not the open key, which the parent handles.
func (k KeyMap) state(m Model) KeyMap {
	notes := len(m.notes.items) > 0 && !m.view.Capturing()
	on := m.OnAnnotations()
	k.Annotations.SetEnabled(k.Annotations.Enabled() && notes)
	k.NotesAnnotations.SetEnabled(k.NotesAnnotations.Enabled() && notes)
	k.NotesAnnotations.SetHelp(k.NotesAnnotations.Help().Key, "log")
	for _, b := range []*key.Binding{&k.Up, &k.Down, &k.Select} {
		b.SetEnabled(b.Enabled() && notes && on)
	}
	k.Open.SetEnabled(false)
	return k
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

// Update takes the view's rests, log and annotations, and passes the rest
// to the log. Keys go to the annotations while they have the focus.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if cmd, ok := m.press(msg); ok {
			return m, cmd
		}
	case notesMsg:
		if msg.id == m.id {
			m.receiveNotes(msg)
		}
		return m, nil
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
	if _, ok := msg.(tea.KeyPressMsg); ok && m.onNotes && m.view.Focused() && !m.view.Capturing() {
		// An option chosen from the annotations leaves the log blurred.
		m.view.Blur()
	}
	return m, cmd
}
