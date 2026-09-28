package statusbar

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// testItems are hints on the left, of which "? help" outranks the rest,
// and on the right an account, a quota that shrinks, and a connection
// that lasts longest.
func testItems() (left, right []Item) {
	left = []Item{
		{Forms: []string{"? help"}, Rank: 3},
		{Forms: []string{"↵ open"}},
		{Forms: []string{"/ search"}},
	}
	right = []Item{
		{Forms: []string{"core 4 812/5 000", "core 96%"}, Rank: 2},
		{Forms: []string{"● online", "●"}, Rank: 4},
		{Forms: []string{"me@github.com"}, Rank: 1},
	}
	return left, right
}

func TestView(t *testing.T) {
	left, right := testItems()
	tests := []struct {
		width int
		want  string
	}{
		{80, " ? help  ↵ open  / search           core 4 812/5 000 · ● online · me@github.com "},
		{60, " ? help         core 4 812/5 000 · ● online · me@github.com "},
		{53, " ? help  core 4 812/5 000 · ● online · me@github.com "},
		{52, " ? help                 core 4 812/5 000 · ● online "},
		{37, " ? help  core 4 812/5 000 · ● online "},
		{30, " ? help   core 96% · ● online "},
		{20, " ? help    ● online "},
		{12, "   ● online "},
		{10, " ● online "},
		{9, "       ● "},
		{3, " ● "},
		{2, "  "},
		{0, ""},
	}
	for _, tt := range tests {
		m := New(WithItems(left, right), WithWidth(tt.width))
		got := ansi.Strip(m.View())
		if got != tt.want {
			t.Errorf("width %d:\n got %q\nwant %q", tt.width, got, tt.want)
		}
		if w := ansi.StringWidth(m.View()); w != tt.width {
			t.Errorf("width %d: the bar is %d wide", tt.width, w)
		}
	}
}

// TestGiveWay checks the order the items give way in as the bar
// narrows: the hints but "? help", the last first, then the account,
// the quota, which shrinks first, "? help", and the connection last,
// which shrinks to its dot first; and that a wider bar never shows less.
func TestGiveWay(t *testing.T) {
	left, right := testItems()
	// Each step is the form of every item, as Shown returns it.
	steps := [][]int{
		{0, 0, 0, 0, 0, 0},
		{0, 0, -1, 0, 0, 0},
		{0, -1, -1, 0, 0, 0},
		{0, -1, -1, 0, 0, -1},
		{0, -1, -1, 1, 0, -1},
		{0, -1, -1, -1, 0, -1},
		{-1, -1, -1, -1, 0, -1},
		{-1, -1, -1, -1, 1, -1},
		{-1, -1, -1, -1, -1, -1},
	}
	step := 0
	for w := 200; w >= 0; w-- {
		m := New(WithItems(left, right), WithWidth(w))
		got := m.Shown()
		for step < len(steps) && !slices.Equal(got, steps[step]) {
			step++
		}
		if step == len(steps) {
			t.Fatalf("width %d shows %v, which isn't a later step than the last", w, got)
		}
	}
	if step != len(steps)-1 {
		t.Errorf("stopped at step %d, want every step down to nothing", step)
	}
}

func TestEmptyForms(t *testing.T) {
	m := New(WithItems([]Item{{Forms: []string{""}}}, []Item{{Forms: []string{"", "x"}}}), WithWidth(10))
	if got, want := m.Shown(), []int{-1, 0}; !slices.Equal(got, want) {
		t.Errorf("Shown() = %v, want %v", got, want)
	}
	if got := ansi.Strip(m.View()); got != strings.Repeat(" ", 8)+"x " {
		t.Errorf("View() = %q", got)
	}
}

func TestViewGolden(t *testing.T) {
	left, right := testItems()
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) {
			m := New(WithStyles(DefaultStyles(dark)), WithItems(left, right), WithWidth(80))
			golden.RequireEqual(t, m.View())
		})
	}
}
