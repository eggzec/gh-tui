package tui

import (
	"testing"
	"testing/synctest"
)

// A digit focuses a pane of the screen on view, or does nothing; it never
// switches screens.
func TestDigitsNeverSwitchScreens(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, false)
		pressKeys(t, m, "S", "1")
		if m.searchHost() == nil {
			t.Errorf("1 on the search page left it for screen %d", m.screen)
		}
		if got := focusOf(m); got != `search: results ""` {
			t.Errorf("1 on the kind strip reached %s, want the results", got)
		}
	})
}
