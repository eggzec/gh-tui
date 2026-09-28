// Package jobview shows one job of a GitHub Actions run: its log, with the
// failed step open on its first error, or its steps while GitHub doesn't
// publish the log yet, and above them the annotations of a failed job,
// each of which opens its file on its line. The Actions modal shows the
// job under the cursor of its jobs with it, and the checks of a pull
// request the job of a check.
//
// A job's log is published only when the job ends, so the view of a job in
// progress shows its steps as they run instead, and the log once the
// parent shows the job again, done.
package jobview

import (
	"context"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
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
}

// KeyMap holds the keys of the view. The parent handles its own keys
// first, so these leave out any it takes.
type KeyMap struct {
	// Log moves through the log.
	Log logview.KeyMap
	// Annotations moves the focus between the annotations and the log.
	Annotations key.Binding
	// Up and Down move through the annotations, and Select opens the file
	// of the one under the cursor.
	Up, Down, Select key.Binding
	// Open opens the job on GitHub, which the notices name.
	Open key.Binding
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
	notes notes
	// focused is set while the view takes keys, and onNotes while the
	// annotations take them rather than the log.
	focused, onNotes bool
	// truncated reports that only the end of the log was read.
	truncated bool
	// resting is set while the log waits for the cursor of the parent to
	// rest on the job before it is read, and seq counts the rests.
	resting bool
	seq     int

	width, height int
	st            ui.RunStyles
}

// Option configures a Model in [New].
type Option func(*options)

type options struct {
	icons ui.Icons
	rest  time.Duration
	now   func() time.Time
	ret   ui.Modal
}

// WithReturn sets the modal that a file preview opened from an annotation
// reopens when it closes: the one the view is shown in.
func WithReturn(m ui.Modal) Option {
	return func(o *options) { o.ret = m }
}

// WithIcons sets the glyphs of the states of the steps. The default is the
// Nerd Font set.
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
	o := options{icons: ui.NewIcons(""), now: time.Now}
	for _, opt := range opts {
		opt(&o)
	}
	m := Model{
		id:   lastID.Add(1),
		ctx:  ctx,
		svc:  svc,
		repo: repo,
		opts: o,
		keys: keys,
		view: logview.New(logview.WithFocusFailed(true), logview.WithKeyMap(keys.Log)),
	}
	return m
}

// SetTheme styles the view.
func (m *Model) SetTheme(t ui.Theme) {
	m.st = ui.NewRunStyles(t, m.opts.icons)
	m.view.SetStyles(t.LogView())
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

// ShortHelp returns the keys of the log for the short help.
func (m Model) ShortHelp() []key.Binding { return m.view.ShortHelp() }

// FullHelp returns the keys of the log for the full help, and those of
// the annotations when there are some.
func (m Model) FullHelp() [][]key.Binding {
	full := m.view.FullHelp()
	if len(m.notes.items) > 0 {
		full = append(full, []key.Binding{m.keys.Up, m.keys.Down, m.keys.Select, m.keys.Annotations})
	}
	return full
}

// Keys returns the main keys of what has the focus, for the short help:
// the moves through the annotations and the one that opens its file, or
// the folds, errors and search of the log. Folding every step works from
// both.
func (m Model) Keys() []key.Binding {
	k, lk := m.keys, m.view.KeyMap()
	if m.OnAnnotations() {
		ann := k.Annotations
		ann.SetHelp(ann.Help().Key, "log")
		return []key.Binding{k.Up, k.Down, k.Select, lk.FoldAll, ann}
	}
	keys := []key.Binding{lk.Toggle, lk.FoldAll, lk.NextError, lk.Search}
	if len(m.notes.items) > 0 {
		keys = append(keys, k.Annotations)
	}
	return keys
}

// State is what the view shows.
func (m Model) State() State { return m.state }

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
	}
	var cmd tea.Cmd
	m.view, cmd = m.view.Update(msg)
	return m, cmd
}
