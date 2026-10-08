package history

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ ui.Actor = (*Modal)(nil)

// Act implements ui.Actor. The quit key closes the modal from any of its
// views; every other intent is the app's to refuse while it is open.
func (m *Modal) Act(action string) (tea.Cmd, bool) {
	if action == ui.ActQuit {
		return m.close(), true
	}
	return nil, false
}
