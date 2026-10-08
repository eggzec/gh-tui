package pulls

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ ui.Actor = (*detailModal)(nil)

// Act implements ui.Actor. The quit key closes the modal from any step of
// it, and the owner key shows the page of the author, as it does without
// the app's help; every other intent is the app's to refuse while the
// modal is open.
func (m *detailModal) Act(action string) (tea.Cmd, bool) {
	switch action {
	case ui.ActQuit:
		return m.close(), true
	case "owner":
		if m.checks != nil || m.ask != nil {
			return nil, false
		}
		return m.author(), true
	}
	return nil, false
}

// author closes the modal and shows the page of the author of the pull
// request, which shows in place of the screen behind it.
func (m *detailModal) author() tea.Cmd {
	if ui.Author(m.detail.Author) == "" {
		return nil
	}
	return tea.Sequence(m.close(), ui.ShowOwner(m.detail.Author.Login))
}
