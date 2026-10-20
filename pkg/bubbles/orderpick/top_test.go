package orderpick

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// The press of the top action, which the root sends when "g g" completes,
// takes the right pane's cursor to its first row.
func TestRightPaneGoesToTheTopOnTheTopAction(t *testing.T) {
	m := open(t, WithSize(80, 5))
	actions := keymap.Actions(m.keys.Picker.Normal.Top)
	if len(actions) == 0 {
		t.Fatal("the top binding names no action")
	}
	m, _ = press(t, m, space, space, space, space, tab)
	if m.top != 3 {
		t.Fatalf("top %d before the action, want 3", m.top)
	}
	m, _ = press(t, m, keymap.ActionPress(actions[0]))
	if m.top != 0 || m.cursor != 0 {
		t.Errorf("top %d, cursor %d after the top action, want 0 and 0", m.top, m.cursor)
	}
}
