package treeview

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// block is a row of n lines, "id 0" to "id n-1".
func block(id string, n int) Node {
	r := Node{ID: id}
	for i := range n {
		r.Lines = append(r.Lines, fmt.Sprintf("%s %d", id, i))
	}
	return r
}

// tallRows is a row of one line, one of ten, and two more of one line.
func tallRows() []Node {
	return []Node{leaf("a", 0), block("T", 10), leaf("b", 0), leaf("c", 0)}
}

func TestTallRowRead(t *testing.T) {
	tests := []struct {
		name    string
		keys    []string
		wantRow string
		wantTop string // the first line of the window
	}{
		{"j enters the row at its top", []string{"j"}, "T", "T 0"},
		{"j scrolls a line", []string{"j", "j"}, "T", "T 1"},
		{"j reaches the bottom", slices.Repeat([]string{"j"}, 7), "T", "T 6"},
		{"j at the bottom leaves the row", slices.Repeat([]string{"j"}, 8), "b", "T 7"},
		{"k enters the row at its bottom", append(slices.Repeat([]string{"j"}, 8), "k"), "T", "T 6"},
		{"k scrolls a line", append(slices.Repeat([]string{"j"}, 8), "k", "k"), "T", "T 5"},
		{"k reaches the top", append(slices.Repeat([]string{"j"}, 8), slices.Repeat([]string{"k"}, 7)...), "T", "T 0"},
		{"k at the top leaves the row", append(slices.Repeat([]string{"j"}, 8), slices.Repeat([]string{"k"}, 8)...), "a", "a"},
		{"ctrl+d scrolls half a page in the row", []string{"j", "ctrl+d"}, "T", "T 2"},
		{"ctrl+d stops at the bottom", []string{"j", "ctrl+d", "ctrl+d", "ctrl+d"}, "T", "T 6"},
		{"ctrl+d leaves once the bottom shows", []string{"j", "ctrl+d", "ctrl+d", "ctrl+d", "ctrl+d"}, "b", "T 7"},
		{"ctrl+d from a row that fits enters the row at its top", []string{"ctrl+d"}, "T", "T 0"},
		{"ctrl+f scrolls a page in the row", []string{"j", "ctrl+f"}, "T", "T 4"},
		{"ctrl+f clamps at the bottom", []string{"j", "ctrl+f", "ctrl+f"}, "T", "T 6"},
		{"ctrl+u scrolls up in the row", []string{"j", "ctrl+f", "ctrl+f", "ctrl+u"}, "T", "T 4"},
		{"ctrl+b leaves the row from its top", []string{"j", "ctrl+b"}, "a", "a"},
		{"ctrl+u into the row shows its bottom", []string{"j", "j", "j", "j", "j", "j", "j", "j", "ctrl+u"}, "T", "T 6"},
		{"end goes to the last row", []string{"j", "G"}, "c", "T 8"},
		{"home shows the top of the first row", []string{"G", "g"}, "a", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTree(tallRows(), WithSize(24, 4))
			m, _ = keys(t, m, tt.keys...)
			if got := selected(m); got != tt.wantRow {
				t.Fatalf("cursor on %q, want %q", got, tt.wantRow)
			}
			got := lines(m)
			if !strings.Contains(got[0], tt.wantTop) {
				t.Fatalf("window starts with %q, want %q:\n%s", got[0], tt.wantTop, strings.Join(got, "\n"))
			}
			assertFits(t, m.View(), 24, 4)
		})
	}
}

// A reveal shows a tall row from its top, whichever side of it the window is
// on, and keeps the window when the cursor is already on the row.
func TestRevealTallRow(t *testing.T) {
	m := newTree(tallRows(), WithSize(24, 4))
	m, _ = keys(t, m, "G")
	m.Reveal("T")
	if got := selected(m); got != "T" {
		t.Fatalf("cursor on %q, want T", got)
	}
	if got := lines(m)[0]; !strings.Contains(got, "T 0") {
		t.Fatalf("window starts with %q, want T 0", got)
	}
	m, _ = keys(t, m, "j", "j")
	want := lines(m)[0]
	m.Reveal("T")
	if got := lines(m)[0]; got != want {
		t.Fatalf("window starts with %q, want %q kept", got, want)
	}
	assertFits(t, m.View(), 24, 4)
}

// A row that fits keeps the moves it had: j and k go row to row.
func TestRowsThatFitMoveByRow(t *testing.T) {
	m := newTree([]Node{block("a", 2), block("b", 2), block("c", 2), block("d", 2)}, WithSize(24, 4))
	m, _ = keys(t, m, "j", "j")
	if got := selected(m); got != "c" {
		t.Fatalf("cursor on %q, want c", got)
	}
	if got := lines(m)[0]; !strings.Contains(got, "b 0") {
		t.Fatalf("window starts with %q, want b 0", got)
	}
	m, _ = keys(t, m, "k", "k")
	if got := selected(m); got != "a" {
		t.Fatalf("cursor on %q, want a", got)
	}
}

// The pins of a thread stay above a tall row inside it.
func TestTallRowWithPins(t *testing.T) {
	rows := []Node{open(fold("root", 0)), block("T", 10), leaf("b", 1)}
	rows[1].Depth = 1
	m := newTree(rows, WithSize(24, 5))
	m, _ = keys(t, m, slices.Repeat([]string{"j"}, 20)...)
	if got := selected(m); got != "b" {
		t.Fatalf("cursor on %q, want b", got)
	}
	m, _ = keys(t, m, "k")
	if got := selected(m); got != "T" {
		t.Fatalf("cursor on %q, want T", got)
	}
	got := lines(m)
	if !strings.Contains(got[0], "root") || !strings.Contains(got[len(got)-1], "T 9") {
		t.Fatalf("want root pinned over the end of T:\n%s", strings.Join(got, "\n"))
	}
	for range 20 {
		m, _ = keys(t, m, "k")
	}
	if got := selected(m); got != "root" {
		t.Fatalf("cursor on %q, want root", got)
	}
	assertShowsOnce(t, m)
}
