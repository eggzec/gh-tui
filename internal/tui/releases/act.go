package releases

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ ui.Actor = (*Modal)(nil)

// Act implements ui.Actor. The quit key closes the modal from any of its
// views, and so does the dismiss key, as nothing in it is transient. The
// back key does nothing, as the modal has no step before it. Every other
// intent is the app's to refuse while it is open.
func (m *Modal) Act(action string) (tea.Cmd, bool) {
	if m.pick != nil && action != ui.ActQuit {
		// The folder picker closes first, and back steps back to the panes.
		if action == ui.ActDismiss || action == ui.ActBack {
			m.pick = nil
			return nil, true
		}
		return nil, false
	}
	switch action {
	case ui.ActQuit, ui.ActDismiss:
		return m.close(), true
	case ui.ActBack:
		return nil, true
	}
	return nil, false
}
