package issues

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ ui.Actor = (*detailModal)(nil)

// Act implements ui.Actor. The quit key closes the modal, the back key
// steps back from the links, and the owner key shows the page of the
// author; every other intent is the app's to refuse while the modal is
// open.
func (m *detailModal) Act(action string) (tea.Cmd, bool) {
	switch action {
	case ui.ActQuit:
		return m.close(), true
	case ui.ActBack:
		// Without links to step back from, the app returns to the modal
		// this one replaced.
		if m.refs == nil || m.refs.TakesKeys() {
			return nil, false
		}
		m.closeRefs()
		return nil, true
	case "owner":
		if m.ask != nil || m.refs != nil || m.composing != composeNone {
			return nil, false
		}
		return m.author(), true
	}
	return nil, false
}

// author closes the modal and shows the page of the author of the issue,
// which shows in place of the screen behind it.
func (m *detailModal) author() tea.Cmd {
	if ui.Author(m.issue.Author) == "" {
		return nil
	}
	return tea.Sequence(m.close(), ui.ShowOwner(m.issue.Author.Login))
}
