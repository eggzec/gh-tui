package pulls

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// TestActOwner checks that the modal takes the owner intent of the keys
// that work everywhere as its author, and leaves the others to the app.
func TestActOwner(t *testing.T) {
	h := started(t, newFakeService(), 80, 20)
	pr, _ := h.feed.Selected()
	press(t, h, "enter")
	m := h.modal()
	if _, ok := m.Act("dashboard"); ok {
		t.Error("the modal took the dashboard intent, which the app refuses over it")
	}
	cmd, ok := m.Act("owner")
	if !ok {
		t.Fatal("the modal didn't take the owner intent")
	}
	if got := drain(t, h, cmd); !slices.Contains(got, tea.Msg(ui.OwnerMsg{Login: pr.Author.Login})) {
		t.Errorf("owner sent %v, want the page of %q", got, pr.Author.Login)
	}
}

// TestActQuit checks that the quit intent closes the modal and cancels its
// reads.
func TestActQuit(t *testing.T) {
	h := started(t, newFakeService(), 80, 20)
	press(t, h, "enter")
	m := h.modal()
	cmd, ok := m.Act(ui.ActQuit)
	if !ok {
		t.Fatal("the modal didn't take the quit intent")
	}
	drain(t, h, cmd)
	if h.modal() != nil || m.ctx.Err() == nil {
		t.Error("quit should close the modal and cancel its reads")
	}
}
