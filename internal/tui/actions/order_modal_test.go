package actions

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/orderpick"
)

// overlay returns the modal the last key opened over the actions modal.
func overlay(tb testing.TB, h *host) ui.Modal {
	tb.Helper()
	for _, msg := range h.take() {
		if o, ok := msg.(ui.OpenModalMsg); ok {
			return o.Modal
		}
	}
	tb.Fatal("no modal opened")
	return nil
}

// choose presses keys in the overlay. What the overlay chooses comes back to
// it, as the app passes it on, and runs the modal's callback.
func choose(h *host, o ui.Modal, ks ...string) {
	for _, k := range ks {
		cmd := o.Update(press(k))
		if cmd == nil {
			continue
		}
		msg := cmd()
		if c, ok := msg.(orderpick.ChosenMsg); ok {
			h.run(o.Update(c))
			continue
		}
		h.handle(msg)
	}
}

func notified(h *host) bool {
	for _, msg := range h.take() {
		if _, ok := msg.(ui.NotifyMsg); ok {
			return true
		}
	}
	return false
}

func TestOpensOnFirstRun(t *testing.T) {
	t.Parallel()
	m, _ := newGroupedModal(t, newFake(), wideW, wideH)
	r, ok := m.runs.Selected()
	if !ok {
		t.Fatal("the cursor is on a header, want the first run")
	}
	if !m.hasRun || m.run.ID != r.ID {
		t.Fatalf("jobs shown for run %d, cursor on %d", m.run.ID, r.ID)
	}
}

func TestGroupModalPicksLevel(t *testing.T) {
	t.Parallel()
	m, h := newGroupedModal(t, newFake(), wideW, wideH)
	if s := screen(m); !strings.Contains(s, "CI (3)") {
		t.Fatalf("no workflow headers:\n%s", s)
	}
	h.keys("g")
	o := overlay(t, h)
	// Event joins the workflow as the second level.
	choose(h, o, "space", "enter")
	s := screen(m)
	if !strings.Contains(s, "push (") || !strings.Contains(s, "CI (") {
		t.Fatalf("no event headers under the workflows:\n%s", s)
	}
}

func TestToggleGroups(t *testing.T) {
	t.Parallel()
	m, h := newGroupedModal(t, newFake(), wideW, wideH)
	h.keys("alt+g")
	if s := screen(m); strings.Contains(s, "CI (3)") {
		t.Fatalf("headers still show:\n%s", s)
	}
	h.keys("alt+g")
	if s := screen(m); !strings.Contains(s, "CI (3)") {
		t.Fatalf("headers did not come back:\n%s", s)
	}
}

func TestNothingToSwitch(t *testing.T) {
	t.Parallel()
	m, h := newModal(t, newFake(), wideW, wideH)
	h.take()
	h.keys("alt+g")
	if !notified(h) {
		t.Error("alt+g with no levels told nothing")
	}
	h.keys("alt+s")
	if !notified(h) {
		t.Error("alt+s with no sort told nothing")
	}
	if strings.Contains(screen(m), "(3)") {
		t.Error("headers showed")
	}
}

func TestSortModalReorders(t *testing.T) {
	t.Parallel()
	m, h := newModal(t, newFake(), wideW, wideH)
	h.keys("s")
	o := overlay(t, h)
	before := paneText(m, runsPane)
	// Workflow, then flip it to Z to A.
	choose(h, o, "space", "tab", "space", "enter")
	after := paneText(m, runsPane)
	if before == after {
		t.Fatalf("sort did not reorder:\n%s", after)
	}
	h.keys("alt+s")
	if got := paneText(m, runsPane); got != before {
		t.Fatalf("alt+s did not restore GitHub's order:\n%s\nwant\n%s", got, before)
	}
	h.keys("alt+s")
	if got := paneText(m, runsPane); got != after {
		t.Fatalf("alt+s did not sort again:\n%s", got)
	}
}

func TestEnterOnHeaderFolds(t *testing.T) {
	t.Parallel()
	m, h := newGroupedModal(t, newFake(), wideW, wideH)
	h.keys("up")
	if _, ok := m.runs.Selected(); ok {
		t.Fatal("up did not reach the header")
	}
	// The feed folds a header with its left and right keys; enter on one does
	// nothing, as drill leaves it to them.
	h.keys("enter")
	if s := screen(m); !strings.Contains(s, "#4812") {
		t.Fatalf("enter on a header changed the list:\n%s", s)
	}
	h.keys("left")
	if s := screen(m); strings.Contains(s, "#4812") || strings.Contains(s, "#4811") {
		t.Fatalf("the group's runs still show:\n%s", s)
	}
	h.keys("right")
	if s := screen(m); !strings.Contains(s, "#4812") {
		t.Fatalf("the group did not open again:\n%s", s)
	}
}

func TestEnterOnRunFoldsJobs(t *testing.T) {
	t.Parallel()
	m, h := newGroupedModal(t, newFake(), wideW, wideH)
	if !strings.Contains(screen(m), "ubuntu-latest") {
		t.Fatal("jobs of the first run do not show")
	}
	h.keys("enter")
	if s := screen(m); strings.Contains(s, "ubuntu-latest") || !strings.Contains(s, "#4812") {
		t.Fatalf("enter on a run did not fold its jobs:\n%s", s)
	}
}

func TestRegroupKeepsTheRun(t *testing.T) {
	t.Parallel()
	m, h := newGroupedModal(t, newFake(), wideW, wideH)
	h.keys("down", "down", "down")
	want, ok := m.runs.Selected()
	if !ok {
		t.Fatal("cursor not on a run")
	}
	h.keys("alt+g")
	got, ok := m.runs.Selected()
	if !ok || got.ID != want.ID || m.run.ID != want.ID {
		t.Fatalf("cursor on run %d (%v), jobs of %d, want %d", got.ID, ok, m.run.ID, want.ID)
	}
}

func TestGroupedRunsView(t *testing.T) {
	t.Parallel()
	m, _ := newGroupedModal(t, newFake(), wideW, wideH)
	v := m.View()
	assertFits(t, ansi.Strip(v), wideW, wideH)
	golden.RequireEqual(t, []byte(v))
}
