package ui

import "testing"

// Look names the look of a row: only a selected row has another look, and
// the focus of its pane tells which.
func TestLook(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		selected, focused bool
		want              int
	}{{false, false, LookPlain}, {false, true, LookPlain}, {true, false, LookBlurred}, {true, true, LookFocused}} {
		if got := Look(c.selected, c.focused); got != c.want {
			t.Errorf("Look(%v, %v) = %d, want %d", c.selected, c.focused, got, c.want)
		}
	}
}
