// Package pager is a less-like viewer for text such as a file: it
// highlights the syntax, scrolls in both directions or soft-wraps, numbers
// the lines, searches them, and filters them.
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
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
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
	// vis are the indices of the lines shown, in order, or nil to show
	// them all. It is replaced, never changed in place. proj is what
	// picked them, and kept how many lines its filter kept; want is what
	// the user asked for, which differs while projecting, as the command
	// that picks the lines runs. pgen counts projections; what an older
	// one picked is dropped. stopProject stops the one running.
	vis         []int32
	proj, want  projection
	kept        int
	projecting  bool
	pgen        int
	stopProject context.CancelFunc

	// gen counts contents; highlights of an older one are dropped. cancel
	// stops the highlighter of the current one.
	gen    int
	cancel context.CancelFunc

	// top and row are the position of the first line in the window, among
	// the lines shown, and, when wrapping, the first of its rows shown. left is the first column shown when not
	// wrapping.
	top, row, left int
	// mark is the line that GoToLine went to, whose number stands out, or
	// -1.
	mark int

	// prompt is the search prompt, open while it is focused.
	prompt cmdline.Model
	// flash is a note on the last key, such as "Pattern not found",
	// shown in place of the name until the next key, as an error unless
	// flashInfo is set.
	flash     string
	flashInfo bool
	// opt reports whether the key bound to Option was pressed, so the next
	// key names the option to toggle.
	opt bool
	// cases is how searches and filters match case.
	cases caseMode
	// num is the count typed before a key, while counting.
	num      int
	counting bool
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
	m := Model{
		settings: s,
		id:       lastID.Add(1),
		prompt:   cmdline.New(cmdline.WithPrompt(promptSearch), cmdline.WithKeyMap(promptKeys())),
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
	m.prompt.SetSize(m.width, 1)
	m.clamp()
}

// Width returns the width.
func (m Model) Width() int { return m.width }

// Height returns the height.
func (m Model) Height() int { return m.height }

// Focus makes the pager react to keys.
func (m *Model) Focus() { m.focused = true }

// Blur makes the pager ignore keys. It closes the prompt, and forgets an
// option or a count it waited for.
func (m *Model) Blur() {
	m.focused = false
	m.opt, m.num, m.counting = false, 0, false
	m.closePrompt()
}

// Focused reports whether the pager reacts to keys.
func (m Model) Focused() bool { return m.focused }

// Capturing reports whether the prompt is open, or the pager waits for
// the name of an option or the key after a count. It then takes every key, so the parent should
// not act on keys of its own.
func (m Model) Capturing() bool { return m.prompt.Focused() || m.opt || m.counting }

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

// ShortHelp implements help.KeyMap. While the prompt is open, it lists
// the keys that close it, and while the pager waits for an option, the
// key that cancels it.
func (m Model) ShortHelp() []key.Binding {
	switch {
	case m.prompt.Focused():
		return []key.Binding{m.confirmKey(), m.keys.Cancel}
	case m.opt:
		return []key.Binding{m.keys.Cancel}
	case m.counting:
		return []key.Binding{m.keys.Home, m.keys.End, m.keys.Cancel}
	}
	return m.keys.ShortHelp()
}

// confirmKey returns the Confirm binding, which says what it does at the
// prompt open.
func (m Model) confirmKey() key.Binding {
	c := m.keys.Confirm
	if m.prompt.Focused() && m.prompt.Prompt() == promptFilter {
		c.SetHelp(c.Help().Key, "filter")
	}
	return c
}

// FullHelp implements help.KeyMap. While the prompt is open, only the keys
// that close it act, and the prompt takes the rest; while the pager waits
// for an option, only the key that cancels it acts, and after a count,
// the keys that go to its line act too.
func (m Model) FullHelp() [][]key.Binding {
	k := m.keys
	k.Confirm = m.confirmKey()
	if m.Capturing() {
		for _, b := range []*key.Binding{
			&k.Up, &k.Down, &k.PageUp, &k.PageDown, &k.HalfPageUp, &k.HalfPageDown, &k.Home, &k.End,
			&k.Left, &k.Right, &k.Option, &k.Search, &k.Filter, &k.Next, &k.Prev, &k.Close,
		} {
			b.SetEnabled(false)
		}
		k.Count.SetEnabled(m.counting)
		k.Home.SetEnabled(m.counting)
		k.End.SetEnabled(m.counting)
	}
	return k.FullHelp()
}
