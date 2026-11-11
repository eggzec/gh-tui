package releases

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// press handles the modal's keys, and passes the others to the focused tree.
func (m *Modal) press(msg tea.KeyPressMsg) tea.Cmd {
	k := &m.keys
	if m.pick != nil {
		return m.pressPick(msg)
	}
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
	switch {
	case keymap.Matches(msg, k.DownloadAll):
		return m.download(m.rel.Assets)
	case keymap.Matches(msg, k.Download), m.nav.Focus() == paneFiles && keymap.Matches(msg, k.Select):
		if a, ok := m.selectedAsset(); ok {
			return m.download([]core.ReleaseAsset{a})
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
