// Package pager is a less-like viewer for text such as a file: it
// highlights the syntax, scrolls in both directions or soft-wraps, numbers
// the lines and searches them.
//
// The content arrives already fetched with [Model.SetContent]; while it is
// on its way, [Model.SetLoading] and [Model.SetError] show a placeholder.
// The pager renders only the lines in its window, so large files stay cheap
// to scroll.
package pager

import (
	"context"
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

var lastID atomic.Int64

// state is what the pager shows in place of lines.
type state int

const (
	// stateEmpty is a pager that was never given content.
	stateEmpty state = iota
	stateLoading
	stateFailed
	stateBinary
	// stateMessage shows a message of the parent, such as why a file isn't
	// shown.
	stateMessage
	stateReady
)

// Model is a pager. Create it with [New]. It starts blurred, and the parent
// focuses it when it is shown.
type Model struct {
	settings

	id      int64
	focused bool

	name  string
	state state
	err   error
	// errText and errHint are what the pager says of err, worded once as
	// it is set.
	errText, errHint string
	// note is the message of stateMessage.
	note string
	spin spinner.Model
	// lines are the lines of the content, cleaned and with tabs expanded,
	// and spans their tokens once the highlighter is done, or nil.
	lines []string
	spans [][]span

	// gen counts contents; highlights of an older one are dropped. cancel
	// stops the highlighter of the current one.
	gen    int
	cancel context.CancelFunc

	// top and row are the first line in the window and, when wrapping,
	// the first of its rows shown. left is the first column shown when not
	// wrapping.
	top, row, left int
	// mark is the line that GoToLine went to, whose number stands out, or
	// -1.
	mark int

	searching bool
	input     textinput.Model
	// search is the search shown and hits its matches in the window. qgen
	// counts searches; what an older one found is dropped. stopSearch
	// stops the one running in the background.
	search     search
	hits       hits
	qgen       int
	stopSearch context.CancelFunc
	// size is the size in bytes of the content shown.
	size int

	// Rendered once in SetStyles, so View only copies them.
	esc      esc
	nameView string
}

// New returns a blurred, empty pager.
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
		mark:     -1,
	}
	m.SetKeyMap(s.keys)
	m.SetStyles(s.styles)
	m.SetSize(s.width, s.height)
	return m
}

// ID returns the instance ID that scopes the pager's messages.
func (m Model) ID() int64 { return m.id }

// Init implements the Elm architecture. A pager has nothing to start until
// it gets content.
func (m Model) Init() tea.Cmd { return nil }

// Name returns the name of the content, as given to SetContent.
func (m Model) Name() string { return m.name }

// Lines returns the number of lines of the content.
func (m Model) Lines() int { return len(m.lines) }

// SetSize sets the width and height, including the status line.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.input.SetWidth(max(m.width-2, 1))
	m.clamp()
}

// Width returns the width.
func (m Model) Width() int { return m.width }

// Height returns the height.
func (m Model) Height() int { return m.height }

// Focus makes the pager react to keys.
func (m *Model) Focus() { m.focused = true }

// Blur makes the pager ignore keys. It closes the search input.
func (m *Model) Blur() {
	m.focused = false
	m.closeSearch()
}

// Focused reports whether the pager reacts to keys.
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
}

// LineNumbers reports whether the line numbers are shown.
func (m Model) LineNumbers() bool { return m.lineNumbers }

// SetLineNumbers sets whether the line numbers are shown.
func (m *Model) SetLineNumbers(show bool) {
	m.lineNumbers = show
	m.clamp()
}

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keys = k
	m.enableSearchKeys()
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
			&k.Left, &k.Right, &k.Wrap, &k.LineNumbers, &k.Search, &k.Next, &k.Prev, &k.Close,
		} {
			b.SetEnabled(false)
		}
	}
	return k.FullHelp()
}
