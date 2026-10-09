package history

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ ui.Actor = (*Modal)(nil)

// Act implements ui.Actor. The quit key closes the modal from any of its
// views, the dismiss key clears what is transient and then closes it, and
// the back key steps back, or does nothing on the branches; every other
// intent is the app's to refuse while it is open.
func (m *Modal) Act(action string) (tea.Cmd, bool) {
	switch action {
	case ui.ActQuit:
		return m.close(), true
	case ui.ActDismiss:
		return m.dismiss(), true
	case ui.ActBack:
		return m.back(), true
	}
	return nil, false
}
