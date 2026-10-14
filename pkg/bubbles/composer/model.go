// Package composer is an input for writing a comment: a titled, multi-line
// text area that wraps to its width, where enter starts a new line and
// another key submits. It opens the text in the user's editor on a key, and
// keeps what was written as a draft under a key the parent gives, so that
// the same composer finds it again, even after a restart.
//
// The composer calls no service: submitting reports the text with
// [SubmitMsg], and the parent sends it, then calls [Model.Discard] so that
// the draft goes. The draft is saved when the composer is closed, when it is
// submitted and when the editor returns, and by [Model.Save], which a
// parent calls before it quits.
package composer

import (
	"os/exec"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
)

// Drafts is where drafts are kept, by key. A draft that is white space only
// is none. Its calls do I/O, so the composer makes them in commands.
type Drafts interface {
	// Load returns the draft under key, or "" if there is none.
	Load(key string) (string, error)
	// Save keeps text as the draft under key.
	Save(key, text string) error
	// Delete removes the draft under key.
	Delete(key string) error
}

// Notes on what the composer refused or couldn't do, shown after the title
// until the next key.
const (
	noteEmpty    = "nothing to submit"
	noteNoEditor = "no editor set"
	noteEditor   = "editor: "
)

// fileName is the name of the file the editor opens, so it knows the text is
// Markdown.
const fileName = "comment.md"

var lastID atomic.Int64

// Model is a composer. Create it with [New]. It starts blurred, and the
// parent focuses it when it is shown, and runs its [Model.Init] to load
// the draft.
type Model struct {
	id    int64
	key   string
	title string
	note  string

	p      prompt.Model
	keys   KeyMap
	drafts Drafts
	// restored is whether the text started as a saved draft.
	restored bool
	// optional is whether a blank text may be submitted.
	optional bool

	editorCmd string
	getenv    func(string) string
	exec      func(*exec.Cmd, tea.ExecCallback) tea.Cmd
	tempDir   string
}

// New returns a blurred composer for the draft under key.
func New(draftKey string, opts ...Option) Model {
	s := defaultSettings()
	for _, opt := range opts {
		opt(&s)
	}
	m := Model{
		id:        lastID.Add(1),
		key:       draftKey,
		title:     s.title,
		keys:      s.keys,
		drafts:    s.drafts,
		optional:  s.optional,
		editorCmd: s.editorCmd,
		getenv:    s.getenv,
		exec:      s.exec,
		tempDir:   s.tempDir,
		p: prompt.New(
			prompt.WithMode(prompt.MultiLine),
			prompt.WithTitle(s.title),
			prompt.WithPlaceholder(s.placeholder),
			prompt.WithSize(s.width, s.height),
			prompt.WithStyles(s.styles),
		),
	}
	m.p.SetKeyMap(m.promptKeys())
	return m
}

// promptKeys returns the keys the text area shows in its hint. The composer
// takes the keys itself before the text area sees them.
func (m Model) promptKeys() prompt.KeyMap {
	return prompt.KeyMap{Submit: m.keys.Submit, Cancel: m.keys.Cancel}
}

// ID returns the instance ID that scopes the composer's messages.
func (m Model) ID() int64 { return m.id }

// Key returns the key of the draft.
func (m Model) Key() string { return m.key }

// Init loads the draft, which [Model.Update] puts in the text if nothing
// was typed before it arrived.
func (m Model) Init() tea.Cmd {
	if m.drafts == nil {
		return nil
	}
	d, dk, id := m.drafts, m.key, m.id
	return func() tea.Msg {
		text, err := d.Load(dk)
		return loadedMsg{id: id, text: text, err: err}
	}
}

// Value returns the text.
func (m Model) Value() string { return m.p.Value() }

// SetValue replaces the text and moves the cursor after it.
func (m *Model) SetValue(v string) { m.p.SetValue(v) }

// Restored reports whether the text started as a saved draft.
func (m Model) Restored() bool { return m.restored }

// SetSize sets the width and height.
func (m *Model) SetSize(width, height int) { m.p.SetSize(width, height) }

// Width returns the width.
func (m Model) Width() int { return m.p.Width() }

// Height returns the height.
func (m Model) Height() int { return m.p.Height() }

// SetStyles sets the styles, those of a prompt.
func (m *Model) SetStyles(s prompt.Styles) { m.p.SetStyles(s) }

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keys = k
	m.p.SetKeyMap(m.promptKeys())
}

// ShortHelp implements help.KeyMap.
func (m Model) ShortHelp() []key.Binding { return m.keys.ShortHelp() }

// FullHelp implements help.KeyMap.
func (m Model) FullHelp() [][]key.Binding { return m.keys.FullHelp() }

// Focus focuses the composer so it takes keys.
func (m *Model) Focus() tea.Cmd { return m.p.Focus() }

// Blur blurs the composer so it ignores keys.
func (m *Model) Blur() { m.p.Blur() }

// Focused reports whether the composer is focused.
func (m Model) Focused() bool { return m.p.Focused() }

// View renders the composer at exactly its width and height.
func (m Model) View() string { return m.p.View() }

// blank reports whether the text can't be submitted.
func (m Model) blank() bool { return strings.TrimSpace(m.Value()) == "" }

// setNote shows note after the title, or none.
func (m *Model) setNote(note string) {
	m.note = note
	title := m.title
	if note != "" {
		title += " · " + note
	}
	m.p.SetTitle(title)
}
