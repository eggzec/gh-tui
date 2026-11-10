package treeview

import (
	"strings"
	"testing"
)

// Lines above the first row the cursor may rest on come back into view when
// the cursor goes back up to that row.
func TestScrollBackUpShowsSkippedTop(t *testing.T) {
	rows := []Node{
		{ID: "m1", Lines: []string{"meta one"}, Skip: true},
		{ID: "m2", Lines: []string{"meta two"}, Skip: true},
		leaf("a", 0), leaf("b", 0), leaf("c", 0), leaf("d", 0), leaf("e", 0), leaf("f", 0),
	}
	m := newTree(rows, WithSize(24, 4))
	if got := strings.TrimSpace(lines(m)[0]); got != "meta one" {
		t.Fatalf("start: first line %q, want the first meta line", got)
	}
	m, _ = keys(t, m, "G")
	// Home would reset the offset itself, so step up with k: the last k has no
	// selectable row above it and only the scroll can bring the top back.
	for range rows {
		m, _ = keys(t, m, "k")
	}
	if got := strings.TrimSpace(lines(m)[0]); got != "meta one" {
		t.Errorf("after stepping up to the first row the first line is %q, want the first meta line", got)
	}
}
