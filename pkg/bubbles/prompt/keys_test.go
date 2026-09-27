package prompt

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, DefaultKeyMap())
	keytest.NoConflicts(t, DefaultKeyMap())
}

// Full help lists both submit keys, and enables enter only where it
// submits.
func TestFullHelpMode(t *testing.T) {
	enabled := func(m Model) []string {
		var out []string
		for _, b := range m.FullHelp()[0] {
			if b.Enabled() {
				out = append(out, b.Help().Key)
			}
		}
		return out
	}
	if got := enabled(New(WithMode(SingleLine))); !slices.Equal(got, []string{"ctrl+s", "enter", "esc"}) {
		t.Errorf("single-line enables %q", got)
	}
	if got := enabled(New()); !slices.Equal(got, []string{"ctrl+s", "esc"}) {
		t.Errorf("multi-line enables %q", got)
	}
}
