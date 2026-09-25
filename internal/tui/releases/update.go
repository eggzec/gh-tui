package releases

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// press handles the modal's keys, and passes the others to the thread.
func (m *Modal) press(msg tea.KeyPressMsg) tea.Cmd {
	k := &m.keys
	switch {
	case key.Matches(msg, k.Back):
		return m.close()
	case key.Matches(msg, k.Open):
		url := m.url
		if m.loaded && m.rel.URL != "" {
			url = m.rel.URL
		}
		if url == "" {
			return nil
		}
		return ui.Open(url)
	case key.Matches(msg, k.Refresh):
		if !m.failed() {
			return nil
		}
		m.err = nil
		return m.get()
	}
	if m.failed() {
		return nil
	}
	var cmd tea.Cmd
	m.thread, cmd = m.thread.Update(msg)
	return cmd
}
