package actions

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

var _ ui.Actor = (*Modal)(nil)

// Act implements ui.Actor. The quit key closes the modal from any of its
// views, the dismiss key clears what is transient and then closes it, and
// the back key steps back a pane, and on the runs is left to the app, which
// returns to the modal this one was opened over, if there is one; every other
// intent is the app's to refuse while it is open.
func (m *Modal) Act(action string) (tea.Cmd, bool) {
	switch action {
	case ui.ActQuit:
		return m.close(), true
	case ui.ActDismiss:
		return m.dismiss(), true
	case ui.ActBack:
		if m.focus == runsPane && !m.notes {
			return nil, false
		}
		return m.back(), true
	}
	return nil, false
}

var _ ui.Actor = (*logModal)(nil)

// Act implements ui.Actor. The quit key closes the log, which returns to the
// run it was opened from; the other intents are the log's own keys or the
// app's.
func (l *logModal) Act(action string) (tea.Cmd, bool) {
	if action == ui.ActQuit {
		return l.close(), true
	}
	return nil, false
}
