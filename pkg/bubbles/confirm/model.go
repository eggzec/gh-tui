package confirm

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Model shows an [Ask] on as many lines as fit its size, and answers it
// from its keys. It holds no state of the answer: the parent closes the
// question once [Model.Answer] says it is done.
type Model struct {
	ask           Ask
	keys          KeyMap
	st            Styles
	width, height int
}

// Option configures a [Model].
type Option func(*Model)

// WithKeyMap sets the keys that answer.
func WithKeyMap(k KeyMap) Option { return func(m *Model) { m.keys = k } }

// WithStyles sets the styles of the line.
func WithStyles(st Styles) Option { return func(m *Model) { m.st = st } }

// New returns the model that asks a. Without a key map every key is
// disabled.
func New(a Ask, opts ...Option) Model {
	m := Model{ask: a, st: DefaultStyles(true)}
	for _, o := range opts {
		o(&m)
	}
	return m
}

// Ask returns what the model asks.
func (m Model) Ask() Ask { return m.ask }

// KeyMap returns the keys that answer.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the keys that answer.
func (m *Model) SetKeyMap(k KeyMap) { m.keys = k }

// SetStyles sets the styles of the line.
func (m *Model) SetStyles(st Styles) { m.st = st }

// SetSize sets the size of the view.
func (m *Model) SetSize(width, height int) { m.width, m.height = max(width, 0), max(height, 0) }

// Answer takes msg as the answer: see [KeyMap.Answer].
func (m Model) Answer(msg tea.KeyPressMsg, armed bool) (cmd tea.Cmd, done bool) {
	return m.keys.Answer(m.ask, msg, armed)
}

// Size returns the size that fits the question and its keys within
// maxWidth and maxHeight: as wide as the question and its keys, or as wide
// as there is room for, and on as many lines as the question wraps to.
func (m Model) Size(maxWidth, maxHeight int) (width, height int) {
	plain, _ := m.ask.keys(m.st, m.keys)
	w := min(maxWidth, ansi.StringWidth(m.ask.Question)+2+ansi.StringWidth(plain))
	return w, min(maxHeight, len(m.ask.Lines(m.st, m.keys, w, MaxLines)))
}

// View renders the question on exactly the size of the model, blank where
// the question leaves room.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	lines := termtext.PadLines(m.ask.Lines(m.st, m.keys, m.width, min(m.height, MaxLines)), m.width, m.height)
	return strings.Join(lines, "\n")
}

// ShortHelp implements help.KeyMap.
func (m Model) ShortHelp() []key.Binding { return m.keys.ShortHelp() }

// FullHelp implements help.KeyMap.
func (m Model) FullHelp() [][]key.Binding { return m.keys.FullHelp() }
