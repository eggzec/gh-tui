package pager

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

// Help lists every key in every state, and enables only those that act:
// the keys that close the search input while it is open, and cancel,
// which takes esc from close, while a search is shown.
func TestHelpState(t *testing.T) {
	enabled := func(m Model) []string {
		var out []string
		for _, r := range keyhelp.Analyze([]keyhelp.Layer{keyhelp.FromHelp("pager", m, false)}) {
			if r.Status != keyhelp.Disabled {
				out = append(out, r.Binding.Help().Key+" "+r.Status.String())
			}
		}
		return out
	}
	m := open(t, "lines.txt", numbered(100), WithSize(40, 11))
	keytest.NoConflicts(t, m)
	if got := enabled(m); len(got) != 15 {
		t.Errorf("idle: %d enabled, want all but the four search keys: %q", len(got), got)
	}
	m, _ = keys(t, m, "/")
	if got := enabled(m); !slices.Equal(got, []string{"enter active", "esc active"}) {
		t.Errorf("searching: enabled %q, want enter and esc", got)
	}
	m = typeText(t, m, "line")
	m, _ = keys(t, m, "enter")
	got := enabled(m)
	for _, want := range []string{"esc active", "q conflict", "n active"} {
		if !slices.Contains(got, want) {
			t.Errorf("search shown: enabled %q, want %q", got, want)
		}
	}
	m, _ = keys(t, m, "esc", "esc", "1")
	if got := enabled(m); !slices.Equal(got, []string{"g/home active", "G/end active", "0-9 active", "esc active"}) {
		t.Errorf("counting: enabled %q, want g, G, the digits and esc", got)
	}
	m, _ = keys(t, m, "esc", "-")
	if got := enabled(m); !slices.Equal(got, []string{"esc active"}) {
		t.Errorf("option: enabled %q, want esc", got)
	}
	m, _ = keys(t, m, "esc")
	keytest.NoConflicts(t, m)
}
