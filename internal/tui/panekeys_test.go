package tui

import (
	"testing"
	"testing/synctest"
)

// A digit focuses a pane of the screen on view, or does nothing; it never
// switches screens.
func TestDigitsNeverSwitchScreens(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newKeysApp(t, false)
		pressKeys(t, m, "I")
		if m.screen != notifScreen {
			t.Fatalf("screen %d after the notifications key, want the notifications", m.screen)
		}
		pressKeys(t, m, "1")
		if m.screen != notifScreen {
			t.Errorf("1 on the notifications opened screen %d", m.screen)
		}
		if m.keys.state(m).Jump.Enabled() {
			t.Error("help offers to focus a pane on the notifications")
		}
		pressKeys(t, m, "I", "S", "tab", "1")
		if m.screen != searchScreen {
			t.Errorf("1 on the search page opened screen %d", m.screen)
		}
		if got := focusOf(m); got != `search: query ""` {
			t.Errorf("1 on the kinds reached %s, want the query", got)
		}
	})
}
