package logview

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, DefaultKeyMap())
	keytest.NoConflicts(t, DefaultKeyMap())
}

// Help enables the search keys only while there is a search to close or
// clear; then cancel takes esc from close.
func TestHelpState(t *testing.T) {
	status := func(m Model, desc string) string {
		for _, r := range keyhelp.Analyze([]keyhelp.Layer{keyhelp.FromHelp("log", m, false)}) {
			if r.Binding.Help().Key == desc {
				return r.Status.String()
			}
		}
		return "missing"
	}
	m := open(t, WithSize(80, 24))
	keytest.NoConflicts(t, m)
	m, _ = keys(t, m, "/")
	keytest.NoConflicts(t, m)
	m = typeText(t, m, "a")
	m, _ = keys(t, m, "enter")
	got := []string{status(m, "enter"), status(m, "esc"), status(m, "q")}
	if !slices.Equal(got, []string{"disabled", "active", "conflict"}) {
		t.Errorf("search shown: confirm, cancel and close are %q", got)
	}
}
