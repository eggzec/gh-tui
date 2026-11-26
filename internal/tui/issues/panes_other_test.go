package issues

import (
	"context"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/slots"
)

func TestOtherKeysScrollThePreviewFromTheList(t *testing.T) {
	t.Parallel()
	h, m := openedBlockers(t, &fakePreviews{body: 80})
	m.View()
	key := slots.Key{Pane: paneBlockers, ID: m.slots.Hover(paneBlockers).ID}
	press(t, h, "J", "J")
	if got := m.slots.Previews().Get(key); got != 2 {
		t.Fatalf("offset = %d after two J, want 2", got)
	}
	press(t, h, "K")
	if got := m.slots.Previews().Get(key); got != 1 {
		t.Fatalf("offset = %d after K, want 1", got)
	}
	if m.nav.InPreview(m.paneLayout()) {
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
	if !m.zoom.Zoomed() {
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
	press(t, h, "j", "j", "j")
	if m.slots.Previews().Get(slots.Key{Pane: paneBlockers, ID: first}) != 3 {
		t.Fatal("the preview didn't scroll")
	}
	press(t, h, "J")
	next := m.slots.Hover(paneBlockers).ID
	if next == first {
		t.Fatalf("J from the preview left the list on %q", first)
	}
	if got := m.slots.Previews().Get(slots.Key{Pane: paneBlockers, ID: next}); got != 0 {
		t.Fatalf("the preview of the row under the cursor starts at %d, want 0", got)
	}
	if !m.nav.InPreview(m.paneLayout()) {
		t.Fatal("J left the preview")
	}
	press(t, h, "K")
	if got := m.slots.Hover(paneBlockers).ID; got != first {
		t.Fatalf("K from the preview hovers %q, want %q", got, first)
	}
	// The group row above has no preview, so the hover stays on the blocker
	// and the place read in its preview is kept.
	press(t, h, "j", "j")
	press(t, h, "K")
	if got := m.slots.Hover(paneBlockers).ID; got != first {
		t.Fatalf("K onto the group row hovers %q, want %q", got, first)
	}
	if got := m.slots.Previews().Get(slots.Key{Pane: paneBlockers, ID: first}); got != 2 {
		t.Fatalf("K that kept the hover moved the preview to %d, want 2", got)
	}
}
