package orderpick

import (
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []any
	}{
		{"wide empty", 80, 10, nil},
		{"wide ticked", 80, 10, []any{letter('j'), space, space}},
		{"wide right in focus", 80, 10, []any{space, space, tab, space, letter('j')}},
		{"wide filtered", 80, 10, []any{letter('/'), letter('c'), enter, space}},
		{"narrow", 40, 16, []any{space, space, tab}},
		{"short", 80, 6, []any{space, space, space}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, WithStyles(asciiStyles()), WithMarks("[x]", "[ ]"), WithSize(tt.width, tt.height))
			for _, k := range tt.keys {
				m, _ = press(t, m, k)
			}
			v := m.View()
			if strings.ContainsFunc(ansi.Strip(v), func(r rune) bool { return r > unicode.MaxASCII }) {
				t.Errorf("view isn't ASCII:\n%s", ansi.Strip(v))
			}
			lines := strings.Split(ansi.Strip(v), "\n")
			if len(lines) != tt.height {
				t.Errorf("view is %d lines, want %d", len(lines), tt.height)
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != tt.width {
					t.Errorf("line %d is %d wide, want %d: %q", i, w, tt.width, l)
				}
			}
			golden.RequireEqual(t, ansi.Strip(v))
		})
	}
}

// The panes stay the size they are given, down to nothing at all.
func TestTinySizes(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {1, 1}, {5, 3}, {59, 4}, {60, 2}, {20, 30}} {
		m := open(t, WithSize(size[0], size[1]))
		m, _ = press(t, m, space, tab)
		v := m.View()
		if size[0] == 0 || size[1] == 0 {
			continue
		}
		if got := strings.Count(v, "\n") + 1; got != size[1] {
			t.Errorf("%v: %d lines, want %d", size, got, size[1])
		}
	}
}
