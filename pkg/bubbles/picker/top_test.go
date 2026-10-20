package picker

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// The press of the top action, which the root sends when "g g" completes,
// takes a picker in normal mode to its first row.
func TestNormalModeGoesToTheTopOnTheTopAction(t *testing.T) {
	items := manyItems(30)
	m := open(t, nil, WithItems(items), WithModes(true))
	actions := keymap.Actions(m.keys.Normal.Top)
	if len(actions) == 0 {
		t.Fatal("the top binding names no action")
	}
	m, _ = press(t, m, letter('G'))
	if got := chosen(t, m); got != items[29].Title {
		t.Fatalf("G selected %q, want the last", got)
	}
	m, _ = press(t, m, keymap.ActionPress(actions[0]))
	if got := chosen(t, m); got != items[0].Title {
		t.Errorf("the top action selected %q, want %q", got, items[0].Title)
	}
}
