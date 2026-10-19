package diff

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// selectionModel is a view of a.go with the cursor on its added line, whose
// rows are: file header, hunk header, context, deleted, added, context.
func selectionModel(t *testing.T) Model {
	t.Helper()
	m := load(t, &source{size: 5, files: []File{goFile("a.go", 1)}}, WithSize(80, 8))
	m = keys(t, m, "down", "down", "down", "down")
	if p, ok := m.Position(); !ok || p.Line != 2 || p.Side != NewSide {
		t.Fatalf("cursor at %+v, %v; want the added line", p, ok)
	}
	return m
}

func selectedFirstLine() Pos { return Pos{Path: "a.go", Side: NewSide, Line: 1} }

func TestSelectionStylesTheRowsBetweenAnchorAndCursor(t *testing.T) {
	t.Parallel()
	m := selectionModel(t)
	if m.Styles().Selected.Render("x") == "x" {
		t.Skip("the test terminal draws no colors")
	}
	plain := strings.Split(m.View(), "\n")

	m.SetAnchor(selectedFirstLine())
	if p, ok := m.Anchor(); !ok || p != selectedFirstLine() {
		t.Fatalf("Anchor = %+v, %v", p, ok)
	}
	view := m.View()
	assertFits(t, view, 80, 8)
	got := strings.Split(view, "\n")
	// Rows 2 to 4 are the context line down to the cursor's line.
	for i := range got {
		same := got[i] == plain[i]
		if inRange := i >= 2 && i <= 4; same == inRange {
			t.Errorf("line %d unchanged = %v, want %v:\n%q", i, same, !inRange, got[i])
		}
	}
	if !slices.Equal(strings.Split(ansi.Strip(view), "\n"), mapStrip(plain)) {
		t.Errorf("a selection changed the text of the view:\n%s", ansi.Strip(view))
	}

	// It follows the cursor: one row down, the last context line joins it.
	m = keys(t, m, "down")
	selected := strings.Split(m.View(), "\n")
	m.ClearAnchor()
	unselected := strings.Split(m.View(), "\n")
	for i := range selected {
		same := selected[i] == unselected[i]
		if inRange := i >= 2 && i <= 5; same == inRange {
			t.Errorf("after down, line %d unchanged = %v, want %v", i, same, !inRange)
		}
	}
}

func TestSameHunk(t *testing.T) {
	t.Parallel()
	m := load(t, &source{size: 5, files: []File{goFile("a.go", 2), goFile("b.go", 1)}}, WithSize(80, 12))
	at := func(path string, line int) Pos { return Pos{Path: path, Side: NewSide, Line: line} }
	for _, tc := range []struct {
		name string
		a, b Pos
		want bool
	}{
		{"one hunk", at("a.go", 1), at("a.go", 3), true},
		{"the same line", at("a.go", 2), at("a.go", 2), true},
		{"two hunks", at("a.go", 3), at("a.go", 21), false},
		{"two hunks, reversed", at("a.go", 21), at("a.go", 1), false},
		{"two files", at("a.go", 1), at("b.go", 1), false},
		{"a line that is not shown", at("a.go", 1), at("a.go", 99), false},
	} {
		if got := m.SameHunk(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: SameHunk = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestClearAnchorEndsTheSelection(t *testing.T) {
	t.Parallel()
	m := selectionModel(t)
	plain := m.View()
	m.SetAnchor(selectedFirstLine())
	m.ClearAnchor()
	if _, ok := m.Anchor(); ok {
		t.Error("Anchor reports a selection after ClearAnchor")
	}
	if got := m.View(); got != plain {
		t.Errorf("view after ClearAnchor differs from the one without a selection:\n%q\n%q", got, plain)
	}
}

func TestNoAnchorLeavesTheViewUnchanged(t *testing.T) {
	t.Parallel()
	m := selectionModel(t)
	if _, ok := m.Anchor(); ok {
		t.Fatal("a new view has an anchor")
	}
	if lo, hi := m.selection(); lo <= hi {
		t.Errorf("selection = %d, %d with no anchor, want an empty range", lo, hi)
	}
	// An anchor on a line the diff does not show selects nothing either.
	plain := m.View()
	m.SetAnchor(Pos{Path: "gone.go", Side: NewSide, Line: 9})
	if got := m.View(); got != plain {
		t.Errorf("an anchor on a line that is not shown changed the view")
	}
}

func mapStrip(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}
