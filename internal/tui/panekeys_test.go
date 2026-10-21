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
		pressKeys(t, m, "S", "tab", "1")
		if m.screen != searchScreen {
			t.Errorf("1 on the search page opened screen %d", m.screen)
		}
		if got := focusOf(m); got != `search: query ""` {
			t.Errorf("1 on the kinds reached %s, want the query", got)
		}
	})
}
