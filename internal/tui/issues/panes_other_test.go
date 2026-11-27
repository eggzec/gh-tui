package issues

import (
	"context"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestOtherKeysScrollThePreviewFromTheList(t *testing.T) {
	t.Parallel()
	h, m := openedBlockers(t, &fakePreviews{body: 80})
	m.View()
	press(t, h, "J")
	m.View()
	press(t, h, "J")
	m.View()
	if got := m.side.zoom.Top(); got != 2 {
		t.Fatalf("offset = %d after two J, want 2", got)
	}
	press(t, h, "K")
	m.View()
	if got := m.side.zoom.Top(); got != 1 {
		t.Fatalf("offset = %d after K, want 1", got)
	}
	if m.ctl.Nav.InPreview(m.paneLayout()) {
		t.Fatal("J moved the focus to the preview")
	}
}

// openedTwoBlockers is openedBlockers for an issue with two blockers, so the
// list has a second row to move to.
func openedTwoBlockers(t *testing.T, f *fakePreviews) (*host, *detailModal) {
	t.Helper()
	svc := newFakeService(sampleIssues(12))
	svc.deps = func(_ context.Context, _ core.RepoRef, n int, kind string) ([]core.Dependency, error) {
		if n == 999 && kind == depBlockedBy {
			return []core.Dependency{
				{Number: 12, Title: "Fix the thing", State: core.StateOpen},
				{Number: 13, Title: "Fix another", State: core.StateOpen},
			}, nil
		}
		if n == 999 {
			return nil, core.ErrNotFound
		}
		return nil, nil
	}
	h := started(t, svc, 120, 30, WithPreviews(f))
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "*", "5")
	if !m.ctl.Zoom.Zoomed() {
		press(t, h, "z")
	}
	m.View()
	press(t, h, "l", "down")
	return h, m
}

func TestOtherKeysMoveTheListFromThePreview(t *testing.T) {
	t.Parallel()
	h, m := openedTwoBlockers(t, &fakePreviews{body: 80})
	m.View()
	first := m.slots.Hover(paneBlockers).ID
	press(t, h, "tab")
	m.View()
	for range 3 {
		press(t, h, "j")
		m.View()
	}
	if m.side.zoom.Top() != 3 {
		t.Fatal("the preview didn't scroll")
	}
	press(t, h, "J")
	next := m.slots.Hover(paneBlockers).ID
	if next == first {
		t.Fatalf("J from the preview left the list on %q", first)
	}
	m.View()
	if got := m.side.zoom.Top(); got != 0 {
		t.Fatalf("the preview of the row under the cursor starts at %d, want 0", got)
	}
	if !m.ctl.Nav.InPreview(m.paneLayout()) {
		t.Fatal("J left the preview")
	}
	press(t, h, "K")
	if got := m.slots.Hover(paneBlockers).ID; got != first {
		t.Fatalf("K from the preview hovers %q, want %q", got, first)
	}
	// The group row above has no preview, so the hover stays on the blocker
	// and the place read in its preview is kept.
	m.View()
	before := m.side.zoom.Top()
	press(t, h, "j")
	m.View()
	press(t, h, "j")
	m.View()
	press(t, h, "K")
	m.View()
	if got := m.slots.Hover(paneBlockers).ID; got != first {
		t.Fatalf("K onto the group row hovers %q, want %q", got, first)
	}
	if got := m.side.zoom.Top(); got != before+2 {
		t.Fatalf("K that kept the hover moved the preview to %d, want %d", got, before+2)
	}
}

func TestZoomedPreviewRouting(t *testing.T) {
	t.Parallel()
	h, m := openedBlockers(t, &fakePreviews{body: 80})
	m.View()
	press(t, h, "tab")
	m.View()
	z := m.side.zoom
	if !m.ctl.Nav.InPreview(m.paneLayout()) || !z.Pager().Focused() {
		t.Fatal("tab didn't give the pager the keys")
	}
	if !z.Wants(keyMsg("j")) || z.Wants(keyMsg("tab")) {
		t.Error("Wants is wrong for j or tab")
	}
	helps := func() (pager, capturing bool) {
		for _, l := range m.KeyLayers() {
			capturing = capturing || l.Context == "search_prompt"
			for _, b := range l.Bindings {
				pager = pager || b.Help().Desc == "down" && b.Enabled()
			}
		}
		return pager, capturing
	}
	if pager, capturing := helps(); !pager || capturing {
		t.Errorf("help in the preview: pager keys %v, prompt %v", pager, capturing)
	}
	press(t, h, "/")
	if !z.Capturing() {
		t.Fatal("/ didn't open the search of the pager")
	}
	if _, capturing := helps(); !capturing {
		t.Error("help doesn't show the search prompt while it is open")
	}
	press(t, h, "tab")
	if !m.ctl.Nav.InPreview(m.paneLayout()) {
		t.Error("tab left the preview while the prompt was open")
	}
	press(t, h, "esc")
	if z.Capturing() || h.modal() == nil {
		t.Fatal("esc didn't close just the prompt")
	}
	press(t, h, "tab")
	m.View()
	if m.ctl.Nav.InPreview(m.paneLayout()) || z.Pager().Focused() {
		t.Error("tab didn't flip back to the list")
	}
}
