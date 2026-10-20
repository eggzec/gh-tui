package filterform

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// The press of the top action, which the root sends when "g g" completes,
// takes the cursor to the first row.
func TestRowsGoToTheTopOnTheTopAction(t *testing.T) {
	m := open(t, prSpec(nil))
	actions := keymap.Actions(m.keys.Top)
	if len(actions) == 0 {
		t.Fatal("the top binding names no action")
	}
	m, _ = press(t, m, keyBigG)
	if m.row == rowState {
		t.Fatal("G didn't leave the first row")
	}
	m, _ = press(t, m, keymap.ActionPress(actions[0]))
	if m.row != rowState {
		t.Errorf("the top action went to row %d, want %d", m.row, rowState)
	}
}
