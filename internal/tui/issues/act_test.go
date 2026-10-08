package issues

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// TestActOwner checks that the modal takes the owner intent of the keys
// that work everywhere as its author, and leaves the others to the app.
func TestActOwner(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(3)), 30)
	if _, ok := m.Act("search"); ok {
		t.Error("the modal took the search intent, which the app refuses over it")
	}
	cmd, ok := m.Act("owner")
	if !ok {
		t.Fatal("the modal didn't take the owner intent")
	}
	if got := run(t, h, cmd); !slices.ContainsFunc(got, func(msg tea.Msg) bool { _, ok := msg.(ui.OwnerMsg); return ok }) {
		t.Errorf("owner sent %v, want the page of the author", got)
	}
}

// TestActQuit checks that the quit intent closes the modal.
func TestActQuit(t *testing.T) {
	h, m := opened(t, newFakeService(sampleIssues(3)), 30)
	cmd, ok := m.Act(ui.ActQuit)
	if !ok {
		t.Fatal("the modal didn't take the quit intent")
	}
	run(t, h, cmd)
	if h.modal() != nil {
		t.Error("quit should close the modal")
	}
}
