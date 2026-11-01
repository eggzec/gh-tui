package treeview

import (
	"slices"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// inlineRows is a review whose body has a text, a block that holds a nested
// block, and a thread, then a comment whose only child is inline.
func inlineRows() []Node {
	return []Node{
		{ID: "r", Branch: true, Lines: []string{"review", "its body"}},
		{ID: "r/t0", Depth: 1, Inline: true, Lines: []string{"text after", "a second line"}},
		{ID: "r/d0", Depth: 1, Inline: true, Branch: true, Marker: "⋯ 2 lines", Lines: []string{"block"}},
		{ID: "r/d0.1", Depth: 2, Inline: true, Branch: true, Closed: true, Marker: "⋯ 1 line", Lines: []string{"nested"}},
		{ID: "r/d0.1/t0", Depth: 3, Inline: true, Lines: []string{"inside"}},
		{ID: "r/d0/end", Depth: 2, Inline: true, Skip: true, Lines: []string{"╰─"}},
		{ID: "t", Depth: 1, Lines: []string{"thread"}},
		{ID: "c", Branch: true, Lines: []string{"comment", "its body"}},
		{ID: "c/t0", Depth: 1, Inline: true, Lines: []string{"a segment", "of the comment"}},
	}
}

// An inline row has no branch of its own: it starts under the body of its
// parent, which carries the guide on only to children that are not inline.
func TestViewInlineRowsHangOffTheParent(t *testing.T) {
	m := newTree(inlineRows(), WithSize(30, 14))
	v := m.View()
	assertFits(t, v, 30, 14)
	golden.RequireEqual(t, plain(v))
}

// A row that is too narrow for its parent's body shows nothing of its text
// instead of overflowing.
func TestViewInlineRowsNarrow(t *testing.T) {
	m := newTree(inlineRows(), WithSize(12, 14))
	v := m.View()
	assertFits(t, v, 12, 14)
	golden.RequireEqual(t, plain(v))
}

func TestInlineRowsFoldWithTheirParent(t *testing.T) {
	m := newTree(inlineRows(), WithSize(30, 14))
	all := []string{"r", "r/t0", "r/d0", "r/d0.1", "r/d0/end", "t", "c", "c/t0"}
	if got := visible(m); !slices.Equal(got, all) {
		t.Fatalf("shows %v, want %v", got, all)
	}
	// A closed fold hides its inline rows and theirs.
	m, _ = keys(t, m, "h")
	if got, want := visible(m), []string{"r", "c", "c/t0"}; !slices.Equal(got, want) {
		t.Fatalf("parent closed: shows %v, want %v", got, want)
	}
	m, _ = keys(t, m, "l", "down", "down", "down", "l")
	if got, want := visible(m), []string{"r", "r/t0", "r/d0", "r/d0.1", "r/d0.1/t0", "r/d0/end", "t", "c", "c/t0"}; !slices.Equal(got, want) {
		t.Fatalf("nested block open: shows %v, want %v", got, want)
	}
}
