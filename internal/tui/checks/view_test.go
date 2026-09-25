package checks

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []string
	}{
		{"190 columns", wideW, wideH, nil},
		{"190 columns job", wideW, wideH, []string{"enter"}},
		{"190 columns running", wideW, wideH, []string{"down", "down", "down", "enter"}},
		{"190 columns detail", wideW, wideH, []string{"down", "down", "enter"}},
		{"190 columns confirm", wideW, wideH, []string{"enter", "ctrl+r"}},
		{"80 columns", narrowW, narrowH, nil},
		{"80 columns job", narrowW, narrowH, []string{"enter"}},
		{"80 columns annotations", narrowW, narrowH, []string{"enter", "A"}},
		{"80 columns detail", narrowW, narrowH, []string{"down", "down", "enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, h := newStep(t, newFake(), tt.width, tt.height)
			h.keys(tt.keys...)
			v := s.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

func TestViewFitsAnySize(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {10, 3}, {30, 5}, {250, 60}} {
		for _, keys := range [][]string{nil, {"enter"}, {"enter", "A"}, {"down", "down", "enter"}, {"enter", "ctrl+r"}} {
			s, h := newStep(t, newFake(), wideW, wideH)
			h.keys(keys...)
			s.SetSize(size[0], size[1])
			assertFits(t, s.View(), size[0], size[1])
		}
	}
}
