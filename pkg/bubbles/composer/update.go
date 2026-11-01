package composer

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/extedit"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// SubmitMsg reports that the user submitted the composer with ID. Text is
// what they wrote, as is, and never blank unless the composer was made
// [WithOptional]. The draft under Key is saved,
// since the send may fail; Err is why that failed, if it did. The parent
// sends the text, then calls [Model.Discard].
type SubmitMsg struct {
	ID   int64
	Key  string
	Text string
	Err  error
}

// CancelMsg reports that the user closed the composer with ID. The text is
// kept as the draft under Key, or the draft is gone if there is none; Err is
// why saving failed, if it did.
type CancelMsg struct {
	ID   int64
	Key  string
	Text string
	Err  error
}

// DiscardedMsg reports that [Model.Discard] removed the draft of the
// composer with ID, or why it couldn't.
type DiscardedMsg struct {
	ID  int64
	Key string
	Err error
}

// loadedMsg carries the draft that Init loaded.
type loadedMsg struct {
	id   int64
	text string
	err  error
}

// editedMsg reports that the editor of the composer with ID is done: the
// text it left, or why it failed.
type editedMsg struct {
	id   int64
	text string
	err  error
}

// Update submits, closes and edits on their keys and passes everything else
// to the text area. A blurred composer ignores keys.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		// A draft that arrives after the user began to write is not put over it.
		if msg.id == m.id && msg.err == nil && m.blank() && strings.TrimSpace(msg.text) != "" {
			m.p.SetValue(msg.text)
			m.restored = true
		}
		return m, nil
	case editedMsg:
		if msg.id != m.id {
			return m, nil
		}
		if msg.err != nil {
			first, _, _ := strings.Cut(msg.err.Error(), "\n")
			m.setNote(noteEditor + termtext.OneLine(first))
			return m, nil
		}
		m.setNote("")
		// Editors end the file with a newline of their own.
		m.p.SetValue(strings.TrimRight(msg.text, "\r\n"))
		return m, m.save()
	case tea.KeyPressMsg:
		if !m.Focused() {
			return m, nil
		}
		if m.note != "" {
			m.setNote("")
		}
		switch {
		case key.Matches(msg, m.keys.Submit):
			if m.blank() && !m.optional {
				m.setNote(noteEmpty)
				return m, nil
			}
			return m, m.finish(true)
		case key.Matches(msg, m.keys.Cancel):
			return m, m.finish(false)
		case key.Matches(msg, m.keys.Edit):
			cmd := m.edit()
			return m, cmd
		}
	}
	var cmd tea.Cmd
	m.p, cmd = m.p.Update(msg)
	return m, cmd
}

// edit opens the text in the editor, or says there is none.
func (m *Model) edit() tea.Cmd {
	words := extedit.Words(m.editorCmd, m.getenv)
	if words == nil {
		m.setNote(noteNoEditor)
		return nil
	}
	id := m.id
	return extedit.Edit(m.exec, words, m.tempDir, fileName, m.Value(), 1,
		func(text string, err error) tea.Msg { return editedMsg{id: id, text: text, err: err} })
}

// finish returns the command that saves the draft and then reports that the
// composer was submitted, or closed.
func (m Model) finish(submit bool) tea.Cmd {
	id, dk, text := m.id, m.key, m.Value()
	d := m.drafts
	return func() tea.Msg {
		err := saveDraft(d, dk, text)
		if submit {
			return SubmitMsg{ID: id, Key: dk, Text: text, Err: err}
		}
		return CancelMsg{ID: id, Key: dk, Text: text, Err: err}
	}
}

// save returns the command that saves the draft, whose message is none.
func (m Model) save() tea.Cmd {
	if m.drafts == nil {
		return nil
	}
	d, dk, text := m.drafts, m.key, m.Value()
	return func() tea.Msg {
		_ = saveDraft(d, dk, text)
		return nil
	}
}

// Save saves the text as the draft now, and waits for the disk. A parent
// calls it before it quits, so that ctrl+c loses nothing (the app reaches it
// through its Saver interface). An earlier save that runs in a command may
// still be in flight, so the two can race on the disk and the last writer
// wins; both write the same text unless the user typed in between.
func (m Model) Save() error { return saveDraft(m.drafts, m.key, m.Value()) }

// Discard returns the command that removes the draft, for the parent to run
// once it sent the text. Its message is a [DiscardedMsg].
func (m Model) Discard() tea.Cmd {
	id, dk, d := m.id, m.key, m.drafts
	return func() tea.Msg {
		var err error
		if d != nil {
			err = d.Delete(dk)
		}
		return DiscardedMsg{ID: id, Key: dk, Err: err}
	}
}

// saveDraft keeps text as the draft under key in d, if there is one. Blank
// text keeps no draft, and drops one kept earlier.
func saveDraft(d Drafts, draftKey, text string) error {
	if d == nil {
		return nil
	}
	if strings.TrimSpace(text) == "" {
		return d.Delete(draftKey)
	}
	return d.Save(draftKey, text)
}
