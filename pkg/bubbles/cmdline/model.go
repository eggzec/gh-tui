// Package cmdline is a command line in the manner of vim's: a prompt such
// as ":" and a one-line input, shown at the bottom of the screen while the
// user types a command. It wraps a bubbles text input, so the usual
// editing keys work, and a line longer than the screen scrolls sideways
// with the cursor in view.
//
// The parent opens it with [Model.Open] and forwards messages to it while
// it is focused. The keys come from its [KeyMap]: by default enter sends a
// [SubmitMsg] with the line, and esc or backspace on an empty line send a
// [CancelMsg]; either way the command line blurs itself, and the parent
// closes it. A parent that quits on ctrl+c should forward it to the
// command line first while it is focused, with ctrl+c among the cancel
// keys, so that ctrl+c cancels the command instead of quitting the
// program.
//
// Given a [Complete] function, it shows the candidates that complete the
// line on a row above it, as vim's wildmenu does, and tab and shift+tab
// insert each in turn.
//
// Up and down recall earlier lines from the history, as in vim: only
// those that start with what was typed. The command line adds each line
// submitted to its history, up to the limit [New] takes, but keeps
// it only in memory: on SubmitMsg the parent saves [Model.History], rather
// than a list of its own, and sets it again in the next session with
// [WithHistory] or [Model.SetHistory].
package cmdline

import (
	"slices"
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// MaxHeight is the most rows a command line takes: the candidate row and
// the line.
const MaxHeight = 2

var lastID atomic.Int64

// Model is a command line. Create it with [New]. It starts blurred, and
// the parent opens it when the user starts a command.
type Model struct {
	id      int64
	focused bool
	input   textinput.Model

	prompt        string
	width, height int
	keys          KeyMap
	styles        Styles

	complete Complete
	comp     completion

	history      []string
	historyLimit int
	walk         walk

	// promptView is the rendered prompt, and promptWidth its width.
	promptView  string
	promptWidth int
	// moreLeft and moreRight are the rendered marks that the candidate row
	// scrolls.
	moreLeft, moreRight string
	// view is rendered whenever the state changes, so View is free.
	view string
}

// New returns a blurred command line whose history keeps the last
// historyLimit lines; past it, the oldest go first. Zero or less keeps
// them all.
func New(historyLimit int, opts ...Option) Model {
	s := settings{
		prompt: ":",
		height: MaxHeight,
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
		id:           lastID.Add(1),
		input:        input,
		prompt:       s.prompt,
		width:        max(s.width, 0),
		height:       max(s.height, 0),
		keys:         s.keys,
		complete:     s.complete,
		historyLimit: max(historyLimit, 0),
		comp:         completion{sel: -1, rowWidth: -1},
	}
	// SetStyles lays the line out, so the value goes in after it and
	// scrolls at the final width.
	m.SetStyles(s.styles)
	m.SetHistory(s.history)
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
	m.walk = walk{}
	m.input.SetValue(initial)
	m.input.CursorEnd()
	return m.Focus()
}

// Focus focuses the command line so it takes keys, keeping its text.
func (m *Model) Focus() tea.Cmd {
	m.focused = true
	cmd := m.input.Focus()
	m.refresh()
	m.render()
	return cmd
}

// Blur blurs the command line so it ignores messages.
func (m *Model) Blur() {
	m.focused = false
	m.input.Blur()
	m.refresh()
	m.render()
}

// Focused reports whether the command line is focused.
func (m Model) Focused() bool { return m.focused }

// Value returns the text of the line, as typed.
func (m Model) Value() string { return m.input.Value() }

// SetValue replaces the text of the line and moves the cursor after it.
func (m *Model) SetValue(v string) {
	m.walk = walk{}
	m.input.SetValue(v)
	m.input.CursorEnd()
	m.refresh()
	m.render()
}

// SetComplete sets the function that completes the line, or nil for no
// completion, and asks it for the candidates of the line.
func (m *Model) SetComplete(f Complete) {
	m.complete = f
	m.refresh()
	m.render()
}

// Candidates returns a copy of the candidates shown.
func (m Model) Candidates() []Candidate { return slices.Clone(m.comp.cands) }

// Selected returns the index in Candidates of the candidate inserted in the
// line, or -1 while the line is as typed.
func (m Model) Selected() int { return m.comp.sel }

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
// lays out around: one for the line, two while it shows candidates above
// it, or none when it has no room. It changes as the user types, so the
// parent lays out again after each Update.
func (m Model) Height() int {
	switch {
	case m.width == 0 || m.height == 0:
		return 0
	case m.height >= 2 && len(m.comp.cands) > 0:
		return 2
	}
	return 1
}

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) { m.keys = k }

// ShortHelp implements help.KeyMap. It leaves out tab without a Complete
// function.
func (m Model) ShortHelp() []key.Binding {
	if m.complete == nil {
		return []key.Binding{m.keys.Submit, m.keys.Cancel}
	}
	return m.keys.ShortHelp()
}

// FullHelp implements help.KeyMap. It disables tab and shift+tab without
// a Complete function, and up and down without a history.
func (m Model) FullHelp() [][]key.Binding {
	k := m.keys
	if m.complete == nil {
		k.Next.SetEnabled(false)
		k.Prev.SetEnabled(false)
	}
	if len(m.history) == 0 {
		k.Older.SetEnabled(false)
		k.Newer.SetEnabled(false)
	}
	return k.FullHelp()
}

func (m *Model) renderPrompt() {
	m.promptView = m.styles.Prompt.Render(oneLine(m.prompt))
	m.promptWidth = ansi.StringWidth(m.promptView)
}
