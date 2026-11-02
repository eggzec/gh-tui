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
		{"j enters the row at its head", []string{"j"}, "T", "a"},
		{"j scrolls a line", []string{"j", "j"}, "T", "T 0"},
		{"j reaches the bottom", slices.Repeat([]string{"j"}, 8), "T", "T 6"},
		{"j at the bottom leaves the row", slices.Repeat([]string{"j"}, 9), "b", "T 7"},
		{"k enters the row at its bottom", append(slices.Repeat([]string{"j"}, 9), "k"), "T", "T 7"},
		{"k scrolls a line", append(slices.Repeat([]string{"j"}, 9), "k", "k"), "T", "T 6"},
		{"k reaches the top", append(slices.Repeat([]string{"j"}, 9), slices.Repeat([]string{"k"}, 8)...), "T", "T 0"},
		{"k at the top leaves the row", append(slices.Repeat([]string{"j"}, 9), slices.Repeat([]string{"k"}, 9)...), "a", "a"},
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
	if got := lines(m)[0]; !strings.Contains(got, "a 1") {
		t.Fatalf("window starts with %q, want a 1", got)
	}
	m, _ = keys(t, m, "k", "k")
	if got := selected(m); got != "a" {
		t.Fatalf("cursor on %q, want a", got)
	}
}

// A row with a Head that the window starts inside keeps only that Head and its
// Right part on the top line, not the text of its first line, and a row
// without one does not.
func TestViewStickyHead(t *testing.T) {
	for _, sticky := range []bool{true, false} {
		rows := tallRows()
		if sticky {
			rows[1].Head, rows[1].Right = "who", "2d"
		}
		m := newTree(rows, WithSize(24, 4))
		m, _ = keys(t, m, slices.Repeat([]string{"j"}, 5)...)
		got := lines(m)
		if has := strings.Contains(got[0], "who"); has != sticky {
			t.Errorf("sticky %v: top line %q", sticky, got[0])
		}
		if sticky && (strings.Contains(got[0], "T 0") || !strings.Contains(got[0], "2d")) {
			t.Errorf("top line %q, want the head and the right part only", got[0])
		}
		if !strings.Contains(got[1], "T 4") {
			t.Errorf("sticky %v: second line %q, want the line it covers to go on", sticky, got[1])
		}
		assertFits(t, m.View(), 24, 4)
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
	if !strings.Contains(got[0], "root") || !strings.Contains(got[len(got)-2], "T 9") {
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
