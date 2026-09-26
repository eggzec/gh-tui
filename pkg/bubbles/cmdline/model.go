// Package cmdline is a command line in the manner of vim's: a prompt such
// as ":" and a one-line input, shown at the bottom of the screen while the
// user types a command. It wraps a bubbles text input, so the usual
// editing keys work, and a line longer than the screen scrolls sideways
// with the cursor in view.
//
// The parent opens it with [Model.Open] and forwards messages to it while
// it is focused. Enter sends a [SubmitMsg] with the line, and esc, ctrl+c
// or backspace on an empty line send a [CancelMsg]; either way the command
// line blurs itself, and the parent closes it. A parent that quits on
// ctrl+c must forward it to the command line first while it is focused,
// so ctrl+c cancels the command instead of quitting the program.
package cmdline

import (
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// MaxHeight is the most rows a command line takes.
const MaxHeight = 1

var lastID atomic.Int64

// Model is a command line. Create it with [New]. It starts blurred, and
// the parent opens it when the user starts a command.
//
// It wraps a bubbles text input, which keeps the text of the line in a
// slice, so two copies of a Model that both go on editing share their
// text. A parent keeps one Model and replaces it with the result of each
// Update, as usual.
type Model struct {
	id      int64
	focused bool
	input   textinput.Model

	prompt        string
	width, height int
	keys          KeyMap
	styles        Styles

	// promptView is the rendered prompt, and promptWidth its width.
	promptView  string
	promptWidth int
	// view is rendered whenever the state changes, so View is free.
	view string
}

// New returns a blurred command line.
func New(opts ...Option) Model {
	s := settings{
		prompt: ":",
		height: MaxHeight,
		keys:   DefaultKeyMap(),
		styles: DefaultStyles(true),
	}
	for _, opt := range opts {
		opt(&s)
	}

	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = oneLine(s.placeholder)
	input.CharLimit = s.charLimit
	// Tab and the arrows are the command line's own.
	off := key.NewBinding(key.WithDisabled())
	input.KeyMap.AcceptSuggestion = off
	input.KeyMap.NextSuggestion = off
	input.KeyMap.PrevSuggestion = off

	m := Model{
		id:     lastID.Add(1),
		input:  input,
		prompt: s.prompt,
		width:  max(s.width, 0),
		height: max(s.height, 0),
		keys:   s.keys,
	}
	// SetStyles lays the line out, so the value goes in after it and
	// scrolls at the final width.
	m.SetStyles(s.styles)
	if s.value != "" {
		m.SetValue(s.value)
	}
	return m
}

// ID returns the instance ID that scopes the command line's messages.
func (m Model) ID() int64 { return m.id }

// Init implements the Elm architecture. The cursor doesn't blink, so there
// is nothing to start.
func (m Model) Init() tea.Cmd { return nil }

// Open focuses the command line with initial as its text and the cursor
// after it, ready for a new command.
func (m *Model) Open(initial string) tea.Cmd {
	m.input.SetValue(initial)
	m.input.CursorEnd()
	return m.Focus()
}

// Focus focuses the command line so it takes keys, keeping its text.
func (m *Model) Focus() tea.Cmd {
	m.focused = true
	cmd := m.input.Focus()
	m.render()
	return cmd
}

// Blur blurs the command line so it ignores messages.
func (m *Model) Blur() {
	m.focused = false
	m.input.Blur()
	m.render()
}

// Focused reports whether the command line is focused.
func (m Model) Focused() bool { return m.focused }

// Value returns the text of the line, as typed.
func (m Model) Value() string { return m.input.Value() }

// SetValue replaces the text of the line and moves the cursor after it.
func (m *Model) SetValue(v string) {
	m.input.SetValue(v)
	m.input.CursorEnd()
	m.render()
}

// Prompt returns the prompt shown before the line.
func (m Model) Prompt() string { return m.prompt }

// SetPrompt sets the prompt shown before the line.
func (m *Model) SetPrompt(p string) {
	m.prompt = p
	m.renderPrompt()
	m.layout()
}

// SetSize sets the width and the most rows the command line may take. It
// takes the rows that [Model.Height] reports, up to height.
func (m *Model) SetSize(width, height int) {
	width, height = max(width, 0), max(height, 0)
	if width == m.width && height == m.height {
		return
	}
	m.width, m.height = width, height
	m.layout()
}

// Width returns the width.
func (m Model) Width() int { return m.width }

// Height returns the rows the command line takes now, which the parent
// lays out around: one for the line, or none when it has no room.
func (m Model) Height() int {
	if m.width == 0 || m.height == 0 {
		return 0
	}
	return 1
}

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) { m.keys = k }

// ShortHelp implements help.KeyMap.
func (m Model) ShortHelp() []key.Binding { return m.keys.ShortHelp() }

// FullHelp implements help.KeyMap.
func (m Model) FullHelp() [][]key.Binding { return m.keys.FullHelp() }

func (m *Model) renderPrompt() {
	m.promptView = m.styles.Prompt.Render(oneLine(m.prompt))
	m.promptWidth = ansi.StringWidth(m.promptView)
}
