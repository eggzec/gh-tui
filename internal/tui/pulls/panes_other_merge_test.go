package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestOtherKeysMoveTheMergeListFromThePreview(t *testing.T) {
	t.Parallel()
	m, _ := mergePanes(t, func(d *core.PullRequestDetail) {
		behindEdit(d)
		d.Draft = true
	})
	press2(m, "z")
	m.View()
	if got := m.slots.Hover(paneMerge).ID; got != "behind" {
		t.Fatalf("hovered %q before any key, want behind", got)
	}
	press2(m, "tab")
	if !m.nav.InPreview(m.layout()) {
		t.Fatal("tab didn't focus the preview")
	}
	// At the top of the list, K has no row to go to, so the preview stays.
	press2(m, "K")
	if got := m.slots.Hover(paneMerge).ID; got != "behind" {
		t.Fatalf("K hovers %q, want behind", got)
	}
	press2(m, "J")
	if got := m.slots.Hover(paneMerge).ID; got != "draft" {
		t.Fatalf("J from the preview hovers %q, want draft", got)
	}
	if !m.nav.InPreview(m.layout()) {
		t.Fatal("J left the preview")
	}
	press2(m, "K")
	if got := m.slots.Hover(paneMerge).ID; got != "behind" {
		t.Fatalf("K from the preview hovers %q, want behind", got)
	}
}
