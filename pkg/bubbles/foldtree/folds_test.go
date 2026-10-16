package foldtree

import (
	"slices"
	"testing"
)

func TestSetFolded(t *testing.T) {
	m := newTree(files(), WithSize(30, 9))
	m, _ = keys(t, m, "j", "j", "j") // flags.go, under cmd and app
	m.SetFolded("cmd", true)
	if !m.Folded("cmd") {
		t.Fatal("cmd is open after SetFolded(cmd, true)")
	}
	if got := selected(m); got != "cmd" {
		t.Errorf("cursor = %q, want the fold that hides its row", got)
	}
	if want := []string{"Files", "cmd", "README.md", "Notes", "todo"}; !slices.Equal(visible(m), want) {
		t.Errorf("visible = %v, want %v", visible(m), want)
	}

	m.SetFolded("cmd", false)
	if m.Folded("cmd") || !slices.Contains(visible(m), "flags.go") {
		t.Errorf("cmd is still closed: %v", visible(m))
	}
	if got := selected(m); got != "cmd" {
		t.Errorf("cursor = %q, want it to stay on cmd", got)
	}

	// A row that is no fold, and a row that is not there, change nothing.
	n := m.Len()
	m.SetFolded("todo", true)
	m.SetFolded("nope", true)
	if m.Len() != n || m.Folded("todo") || m.Folded("nope") {
		t.Errorf("SetFolded on a leaf or an unknown ID changed the tree: %v", visible(m))
	}
}

// h walks up to the nearest ancestor, and a parent that the cursor may not rest
// on is passed over to its own parent, never to a sibling of it.
func TestCollapseOverASkipParent(t *testing.T) {
	skip := open(fold("skipped", 1))
	skip.Skip = true
	rows := []Row{
		open(fold("grand", 0)),
		leaf("sibling", 1),
		skip,
		leaf("child", 2),
	}
	m := newTree(rows, WithSize(30, 6))
	m, _ = keys(t, m, "G")
	if got := selected(m); got != "child" {
		t.Fatalf("cursor = %q, want child", got)
	}
	m, _ = keys(t, m, "h")
	if got := selected(m); got != "grand" {
		t.Errorf("cursor after h = %q, want grand", got)
	}
}

func TestRevealStaysOffASkipRow(t *testing.T) {
	rule := leaf("rule", 0)
	rule.Skip = true
	m := newTree([]Row{leaf("a", 0), rule, leaf("b", 0)}, WithSize(10, 4))
	if !m.Reveal("rule") {
		t.Fatal("Reveal(rule) = false")
	}
	if got := selected(m); got == "rule" {
		t.Errorf("cursor rests on a row it may not rest on")
	}
}
