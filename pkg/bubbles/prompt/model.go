// Package prompt is a titled input panel for writing a comment or editing a
// short value, such as a list of labels. It wraps a bubbles text area in
// multi-line mode and a text input in single-line mode, and reports the
// result with [SubmitMsg] or [CancelMsg].
package prompt

import (
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Mode is whether a prompt takes one line or many.
type Mode int

// The modes.
const (
	// MultiLine edits text over many lines in a text area, where enter
	// starts a new line.
	MultiLine Mode = iota
	// SingleLine edits one line in a text input, scrolling sideways.
	SingleLine
)

// SingleLineHeight is the height a single-line prompt needs for its title,
// its input and the hint. A taller one leaves the extra rows blank.
const SingleLineHeight = 3

var lastID atomic.Int64

// Model is a prompt. Create it with [New]. It starts blurred, and the parent
// focuses it when it is shown.
type Model struct {
	id      int64
	mode    Mode
	title   string
	focused bool

	area  textarea.Model
	input textinput.Model

	width, height int
	keys          KeyMap
	styles        Styles
	// The frame's sides, rendered in SetStyles.
	focusedEdges, blurredEdges edges

	// hint is the key hint under the input, rendered when the keys or
	// styles change.
	hint string
	// view is rendered whenever the state changes, so View is free.
	view string
}

// New returns a blurred prompt.
func New(opts ...Option) Model {
	s := settings{
		keys:   DefaultKeyMap(),
		styles: DefaultStyles(true),
	}
	for _, opt := range opts {
		opt(&s)
	}
	if s.mode != SingleLine {
		s.mode = MultiLine
	}

	area := textarea.New()
	area.Prompt = ""
	area.ShowLineNumbers = false
	area.MaxHeight, area.MaxWidth = 0, 0
	area.CharLimit = s.charLimit
	area.Placeholder = s.placeholder

	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = s.charLimit
	input.Placeholder = s.placeholder

	m := Model{
		id:     lastID.Add(1),
		mode:   s.mode,
		title:  s.title,
		area:   area,
		input:  input,
		width:  max(s.width, 0),
		height: max(s.height, 0),
		keys:   s.keys,
	}
	// SetStyles lays the prompt out, so the value goes in after it and
	// wraps at the final width.
	m.SetStyles(s.styles)
	if s.value != "" {
		m.SetValue(s.value)
	}
	return m
}

// ID returns the instance ID that scopes the prompt's messages.
func (m Model) ID() int64 { return m.id }

// Init implements the Elm architecture. The cursor doesn't blink, so there
// is nothing to start.
func (m Model) Init() tea.Cmd { return nil }

// Mode returns whether the prompt takes one line or many.
func (m Model) Mode() Mode { return m.mode }

// Value returns the text in the input.
func (m Model) Value() string {
	if m.mode == SingleLine {
		return m.input.Value()
	}
	return m.area.Value()
}

// SetValue replaces the text in the input and moves the cursor after it.
func (m *Model) SetValue(v string) {
	if m.mode == SingleLine {
		m.input.SetValue(v)
		m.input.CursorEnd()
	} else {
		m.area.SetValue(v)
	}
	m.render()
}

// Title returns the title.
func (m Model) Title() string { return m.title }

// SetTitle sets the title.
func (m *Model) SetTitle(title string) {
	m.title = title
	m.render()
}

// SetSize sets the width and height.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.layout()
}

// Width returns the width.
func (m Model) Width() int { return m.width }

// Height returns the height.
func (m Model) Height() int { return m.height }

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keys = k
	m.renderHint()
	m.render()
}

// ShortHelp implements help.KeyMap. A single-line prompt offers enter to
// submit, and a multi-line one ctrl+s.
func (m Model) ShortHelp() []key.Binding {
	if m.mode == SingleLine {
		return []key.Binding{m.keys.SubmitLine, m.keys.Cancel}
	}
	return m.keys.ShortHelp()
}

// FullHelp implements help.KeyMap. A multi-line prompt disables
// SubmitLine, since enter starts a new line there.
func (m Model) FullHelp() [][]key.Binding {
	k := m.keys
	if m.mode != SingleLine {
		k.SubmitLine.SetEnabled(false)
	}
	return k.FullHelp()
}

// Focus focuses the prompt so it takes keys.
func (m *Model) Focus() tea.Cmd {
	m.focused = true
	var cmd tea.Cmd
	if m.mode == SingleLine {
		cmd = m.input.Focus()
	} else {
		cmd = m.area.Focus()
	}
	// The frames of the two states may differ in width.
	m.layout()
	return cmd
}

// Blur blurs the prompt so it ignores keys.
func (m *Model) Blur() {
	m.focused = false
	m.input.Blur()
	m.area.Blur()
	m.layout()
}

// Focused reports whether the prompt is focused.
func (m Model) Focused() bool { return m.focused }
