package issues

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// infoRows are the heights of Involved, Development and Blockers.
func infoRows(t *testing.T, m *detailModal) (inv, dev, block int) {
	t.Helper()
	rows := func(p panes.Pane) int {
		b, ok := m.paneLayout().Box(p, panes.Whole)
		if !ok {
			t.Fatalf("no box for pane %d", p)
		}
		return b.Rect.H
	}
	return rows(paneInvolved), rows(paneDevelopment), rows(paneBlockers)
}

func TestInfoGrowsFromTheNeighbour(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		focus string
		up    [3]int // change in each pane after ctrl+up
		cut   [2]bool
	}{
		{"Involved takes from Development", "3", [3]int{2, -2, 0}, [2]bool{true, false}},
		{"Development takes from Blockers", "4", [3]int{0, 2, -2}, [2]bool{false, true}},
		{"Blockers takes from Development", "5", [3]int{0, -2, 2}, [2]bool{false, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, m, d := dividedModal(t, 160, false)
			press(t, h, tt.focus)
			a, b, c := infoRows(t, m)
			press(t, h, "ctrl+up")
			a2, b2, c2 := infoRows(t, m)
			if got := [3]int{a2 - a, b2 - b, c2 - c}; got != tt.up {
				t.Errorf("ctrl+up changed the panes by %v, want %v", got, tt.up)
			}
			if got := [2]bool{d.Info[0].Moved(), d.Info[1].Moved()}; got != tt.cut {
				t.Errorf("moved dividers %v, want %v", got, tt.cut)
			}
			press(t, h, "ctrl+down")
			if a3, b3, c3 := infoRows(t, m); a3 != a || b3 != b || c3 != c {
				t.Errorf("ctrl+down did not undo ctrl+up: %d %d %d, want %d %d %d", a3, b3, c3, a, b, c)
			}
		})
	}
}

func TestInfoStopsAtMinimum(t *testing.T) {
	t.Parallel()
	for _, focus := range []string{"3", "4", "5"} {
		for _, dir := range []string{"ctrl+up", "ctrl+down"} {
			h, m, d := dividedModal(t, 160, false)
			press(t, h, focus)
			for range 80 {
				press(t, h, dir)
			}
			a, b, c := infoRows(t, m)
			for _, rows := range []int{a, b, c} {
				if rows < panes.MinHeight {
					t.Fatalf("focus %s %s: panes are %d %d %d, below the minimum", focus, dir, a, b, c)
				}
			}
			if a+b+c != m.paneLayout().Height {
				t.Fatalf("focus %s %s: panes are %d %d %d, do not fill the column", focus, dir, a, b, c)
			}
			before := slices.Clone(d.Info[:])
			press(t, h, dir)
			if after := d.Info[:]; !slices.Equal(before, after) {
				t.Errorf("focus %s %s: a press at the limit moved a divider", focus, dir)
			}
		}
	}
}

func TestInfoSettlesOnResize(t *testing.T) {
	t.Parallel()
	h, m, d := dividedModal(t, 160, false)
	press(t, h, "3")
	for range 80 {
		press(t, h, "ctrl+up")
	}
	m.SetSize(160, 24)
	a, b, c := infoRows(t, m)
	if b != panes.MinHeight || c < panes.MinHeight || a+b+c != m.paneLayout().Height {
		t.Errorf("panes are %d %d %d after resize", a, b, c)
	}
	press(t, h, "ctrl+down")
	if a2, _, _ := infoRows(t, m); a2 >= a {
		t.Errorf("a press after resize did not shrink Involved: %d, was %d", a2, a)
	}
	if !d.Moved() {
		t.Error("the offset was lost")
	}
}

func TestInfoResetsWithTheOthers(t *testing.T) {
	t.Parallel()
	h, m, d := dividedModal(t, 160, false)
	a, b, c := infoRows(t, m)
	press(t, h, "3", "ctrl+up", "5", "ctrl+up", "=")
	if a2, b2, c2 := infoRows(t, m); a2 != a || b2 != b || c2 != c || d.Moved() {
		t.Errorf("after reset the panes are %d %d %d, want %d %d %d", a2, b2, c2, a, b, c)
	}
}

func TestInfoKeysDoNothingWithoutAStack(t *testing.T) {
	t.Parallel()
	h, m, d := dividedModal(t, 100, false)
	press(t, h, "3", "ctrl+up")
	if d.Moved() || m.canStack() {
		t.Error("a narrow modal moved a side divider")
	}
	h, m, d = dividedModal(t, 160, false)
	press(t, h, "3", "z", "ctrl+up")
	if d.Moved() || m.canStack() {
		t.Error("a zoomed pane moved a side divider")
	}
}

func TestInfoUnmovedCutsAreThirds(t *testing.T) {
	t.Parallel()
	for _, h := range []int{9, 10, 11, 30, 31} {
		first, second := infoCuts(&ui.Dividers{Step: 2}, h)
		nf, ns := infoCuts(nil, h)
		if first != h/3 || second != 2*(h/3) || nf != first || ns != second {
			t.Errorf("h=%d: cuts %d %d (nil %d %d), want %d %d", h, first, second, nf, ns, h/3, 2*(h/3))
		}
	}
}
