package releases

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// press handles the modal's keys, and passes the others to the focused tree.
func (m *Modal) press(msg tea.KeyPressMsg) tea.Cmd {
	k := &m.keys
	switch {
	case keymap.Matches(msg, k.Dismiss):
		return m.close()
	case keymap.Matches(msg, k.Refresh):
		if !m.failed() {
			return nil
		}
		m.err = nil
		return m.get()
	}
	if m.failed() {
		return nil
	}
	if m.pressPane(msg) {
		return nil
	}
	if keymap.Matches(msg, k.Open) {
		if url := m.Link(); url != "" {
			return ui.Open(url)
		}
		return nil
	}
	var cmd tea.Cmd
	if m.nav.Focus() == paneFiles {
		m.files, cmd = m.files.Update(msg)
	} else {
		m.notes, cmd = m.notes.Update(msg)
	}
	return cmd
}
