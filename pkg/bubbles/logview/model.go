// Package logview shows the log of a job, such as a GitHub Actions job: its
// steps and groups fold like a tree, errors and warnings are a key away,
// and the colors of tool output are kept while every other escape sequence
// is stripped.
//
// The log arrives already fetched and parsed, as [Line] values and optional
// [Section] values for the steps, with [Model.SetLines]. A job that is still
// running grows with [Model.Append]. The view renders only the rows in its
// window, so logs of a hundred thousand lines stay cheap to scroll.
package logview

import (
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

var lastID atomic.Int64

// state is what the view shows in place of the log.
type state int

const (
	// stateEmpty is a view that was never given a log.
	stateEmpty state = iota
	stateLoading
	stateFailed
	stateReady
)

// Model is a log view. Create it with [New]. It starts blurred, and the
// parent focuses it when it is shown.
type Model struct {
	settings

	id      int64
	focused bool

	title string
	state state
	err   error
	spin  spinner.Model

	content

	// cur is the entry of vis under the cursor. top and row are the first
	// entry in the window and, when wrapping, the first of its rows shown.
	// left is the first column of text shown when not wrapping.
	cur, top, row, left int
	// live is set once lines were appended, which makes the status line
	// show whether the view follows them.
	live bool

	searching bool
	input     textinput.Model
	search    search
	// jumped is the list the last e, E, w or W moved in, and at is the
	// entry of it the cursor went to.
	jumped Kind
	at     int

	// Rendered once in SetStyles, so View only copies them.
	esc       esc
	titleView string
}

// New returns a blurred, empty log view.
func New(opts ...Option) Model {
	s := defaultSettings()
	for _, opt := range opts {
		opt(&s)
	}
	input := textinput.New()
	input.Prompt = "/"
	m := Model{
		settings: s,
		id:       lastID.Add(1),
		input:    input,
		spin:     spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	m.SetKeyMap(s.keys)
	m.SetStyles(s.styles)
	m.SetSize(s.width, s.height)
	return m
}

// ID returns the instance ID that scopes the view's messages.
func (m Model) ID() int64 { return m.id }

// Init implements the Elm architecture. A log view has nothing to start
// until it gets a log.
func (m Model) Init() tea.Cmd { return nil }

// Title returns the title shown in the status line.
func (m Model) Title() string { return m.title }

// SetTitle sets the title shown in the status line, such as the name of the
// job.
func (m *Model) SetTitle(title string) {
	m.title = title
	m.renderTitle()
}

// SetSize sets the width and height, including the status line.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.input.SetWidth(max(m.width-2, 1))
	m.clamp()
	m.show()
}

// Width returns the width.
func (m Model) Width() int { return m.width }

// Height returns the height.
func (m Model) Height() int { return m.height }

// Focus makes the view react to keys.
func (m *Model) Focus() { m.focused = true }

// Blur makes the view ignore keys. It closes the search input.
func (m *Model) Blur() {
	m.focused = false
	m.closeSearch()
}

// Focused reports whether the view reacts to keys.
func (m Model) Focused() bool { return m.focused }

// Capturing reports whether the search input is open. It then takes every
// key, so the parent should not act on keys of its own.
func (m Model) Capturing() bool { return m.searching }

// Wrap reports whether long lines are soft-wrapped.
func (m Model) Wrap() bool { return m.wrap }

// SetWrap sets whether long lines are soft-wrapped instead of scrolled
// sideways.
func (m *Model) SetWrap(wrap bool) {
	m.wrap = wrap
	m.row, m.left = 0, 0
	m.clamp()
	m.show()
}

// LineNumbers reports whether the line numbers are shown.
func (m Model) LineNumbers() bool { return m.lineNumbers }

// SetLineNumbers sets whether the line numbers are shown.
func (m *Model) SetLineNumbers(show bool) {
	m.lineNumbers = show
	m.clamp()
	m.show()
}

// TimeMode returns how the time of each line is shown.
func (m Model) TimeMode() TimeMode { return m.times }

// SetTimeMode sets how the time of each line is shown.
func (m *Model) SetTimeMode(t TimeMode) {
	m.times = t
	m.clamp()
	m.show()
}

// Follow reports whether the view follows appended lines while its cursor
// is on the last line.
func (m Model) Follow() bool { return m.follow }

// SetFollow sets whether the view follows appended lines while its cursor
// is on the last line. Turning it on moves the cursor there.
func (m *Model) SetFollow(follow bool) {
	m.follow = follow
	if follow {
		m.end()
	}
}

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keys = k
	m.enableKeys()
}

// ShortHelp implements help.KeyMap. While the search input is open, it
// lists the keys that close it.
func (m Model) ShortHelp() []key.Binding {
	if m.searching {
		return []key.Binding{m.keys.Confirm, m.keys.Cancel}
	}
	return m.keys.ShortHelp()
}

// FullHelp implements help.KeyMap. While the search input is open, only
// the keys that close it act, and the input takes the rest.
func (m Model) FullHelp() [][]key.Binding {
	k := m.keys
	if m.searching {
		for _, b := range []*key.Binding{
			&k.Up, &k.Down, &k.PageUp, &k.PageDown, &k.HalfPageUp, &k.HalfPageDown, &k.Home, &k.End,
			&k.Left, &k.Right, &k.Toggle, &k.Expand, &k.Collapse, &k.FoldAll,
			&k.NextError, &k.PrevError, &k.NextWarning, &k.PrevWarning,
			&k.Wrap, &k.Times, &k.LineNumbers, &k.Follow, &k.Search, &k.Next, &k.Prev, &k.Close,
		} {
			b.SetEnabled(false)
		}
	}
	return k.FullHelp()
}

// enableKeys enables the keys that move between errors, warnings and
// matches only while there are some, so help shows them only when they
// work, and the search keys while there is a search to close or clear.
func (m *Model) enableKeys() {
	m.keys.NextError.SetEnabled(len(m.errs) > 0)
	m.keys.PrevError.SetEnabled(len(m.errs) > 0)
	m.keys.NextWarning.SetEnabled(len(m.warns) > 0)
	m.keys.PrevWarning.SetEnabled(len(m.warns) > 0)
	found := len(m.search.matches) > 0
	m.keys.Next.SetEnabled(found)
	m.keys.Prev.SetEnabled(found)
	m.keys.Confirm.SetEnabled(m.searching)
	m.keys.Cancel.SetEnabled(m.searching || m.search.query != "")
}
