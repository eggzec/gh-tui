package treeview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func indentStyles(width int) Styles {
	s := DefaultStyles(true)
	s.Guides, s.IndentWidth = Indent, width
	return s
}

func TestViewIndentGuides(t *testing.T) {
	m := newTree(files(), WithSize(30, 9), WithStyles(indentStyles(2)))
	v := m.View()
	assertFits(t, v, 30, 9)
	golden.RequireEqual(t, plain(v))
}

func TestViewIndentWidth(t *testing.T) {
	m := newTree(files(), WithSize(30, 9), WithStyles(indentStyles(4)))
	v := m.View()
	assertFits(t, v, 30, 9)
	golden.RequireEqual(t, plain(v))
}

func TestPrefix(t *testing.T) {
	tests := []struct {
		name  string
		s     Styles
		depth int
		last  bool
		more  []bool
		want  string
	}{
		{"depth 0 has none", DefaultStyles(true), 0, false, nil, ""},
		{"connectors last", DefaultStyles(true), 1, true, nil, plainOf(DefaultStyles(true).Last)},
		{"indent is blank", indentStyles(2), 3, false, []bool{false, true, true}, "      "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.TrimRight(plain(Prefix(tt.s, tt.depth, tt.last, tt.more)), " ")
			if want := strings.TrimRight(tt.want, " "); got != want {
				t.Errorf("Prefix = %q, want %q", got, want)
			}
		})
	}
}

func plainOf(s string) string { return strings.TrimRight(s, " ") }

// A tree with a fold keeps a two-cell arrow slot on every row, blank on
// leaves, so that siblings line up; a tree with none keeps no slot.
func TestArrowSlot(t *testing.T) {
	withFold := newTree([]Node{fold("a", 0), leaf("b", 0)}, WithSize(12, 2), WithFocused(false))
	got := strings.Split(plain(withFold.View()), "\n")
	if want := "▌▸ a"; strings.TrimRight(got[0], " ") != want {
		t.Errorf("fold row = %q, want %q", got[0], want)
	}
	if want := "   b"; strings.TrimRight(got[1], " ") != want {
		t.Errorf("leaf row = %q, want %q", got[1], want)
	}
	flat := newTree([]Node{leaf("a", 0), leaf("b", 0)}, WithSize(12, 2), WithFocused(false))
	got = strings.Split(plain(flat.View()), "\n")
	if want := "▌a"; strings.TrimRight(got[0], " ") != want {
		t.Errorf("flat row = %q, want %q", got[0], want)
	}
}
