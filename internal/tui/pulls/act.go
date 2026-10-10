package pulls

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ ui.Actor = (*detailModal)(nil)

// Act implements ui.Actor. The quit key closes the modal from any step of
// it, the back key steps back from the links, or out of what the Checks
// tab opened, and the owner key shows the page of the author; every other
// intent is the app's to refuse while the modal is open.
func (m *detailModal) Act(action string) (tea.Cmd, bool) {
	switch action {
	case ui.ActQuit:
		return m.close(), true
	case ui.ActBack:
		// The links step back to the tab they were opened over. Otherwise
		// the app returns to the modal this one replaced, unless the
		// Checks tab shows a log, annotations or a detail to step out of.
		if m.refs != nil {
			if m.refs.TakesKeys() {
				return nil, false
			}
			return m.closeRefs(), true
		}
		if m.onChecks() && !m.checks.TakesKeys() && m.checks.StepOut() {
			return nil, true
		}
		return nil, false
	case "owner":
		if m.ask != nil || m.refs != nil || m.find != nil || m.onChecks() && m.checks.TakesKeys() {
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
