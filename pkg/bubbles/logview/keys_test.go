package logview

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, testKeys(t))
	keytest.Tagged(t, testKeys(t))
	keytest.HelpTags(t, testKeys(t))
	keytest.NoConflicts(t, testKeys(t))
}

// Help enables the search keys only while there is a search to close or
// clear; then cancel takes esc from close.
func TestHelpState(t *testing.T) {
	// enter also folds, so a key is told apart by what it does.
	status := func(m Model, k, desc string) string {
		for _, r := range keyhelp.Analyze([]keyhelp.Layer{keyhelp.FromHelp("log", m, false)}) {
			if h := r.Binding.Help(); h.Key == k && h.Desc == desc {
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
	got := []string{status(m, "↵", "search"), status(m, "esc", "cancel"), status(m, "q/esc", "close")}
	if !slices.Equal(got, []string{"disabled", "active", "conflict"}) {
		t.Errorf("search shown: confirm, cancel and close are %q", got)
	}
}

// Quit and dismiss both close the view, and help lists them as one row.
func TestQuitAndDismiss(t *testing.T) {
	look := lookup
	noQuit := func(action string) []string {
		if action == "global.quit" {
			return nil
		}
		return look(action)
	}
	m := open(t, WithSize(80, 24), WithKeyMap(NewKeyMap(noQuit)))
	if _, msg := keys(t, m, "q"); msg != nil {
		t.Errorf("q sent %v, though quit has no key", msg)
	}
	if _, msg := keys(t, m, "esc"); msg == nil {
		t.Error("esc didn't close")
	}
	last := m.FullHelp()[4]
	if b := last[len(last)-1]; b.Help().Key != "esc" || b.Help().Desc != "close" {
		t.Errorf("help lists %q %q last, want one row esc close", b.Help().Key, b.Help().Desc)
	}
	m = open(t, WithSize(80, 24))
	if _, msg := keys(t, m, "q"); msg == nil {
		t.Error("q didn't close")
	}
}

// A view given no key map has no key bound.
func TestNoKeyMap(t *testing.T) {
	lines, secs := fixture()
	m := New(WithSize(80, 24))
	m.Focus()
	m.SetLines(lines, secs)
	if _, msg := keys(t, m, "q", "esc", "j", "/"); msg != nil || m.Capturing() {
		t.Errorf("a key did something: sent %v, capturing %v", msg, m.Capturing())
	}
}
