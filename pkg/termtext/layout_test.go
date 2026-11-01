package termtext

import (
	"slices"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSpread(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
		w           int
		want        string
	}{
		{"both fit", "ask", "y/n", 10, "ask    y/n"},
		{"left cut", "a long question", "y/n", 10, "a lon… y/n"},
		{"no right", "ask", "", 6, "ask   "},
		{"right too wide", "ask", "y/n", 4, "ask "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Spread(tt.left, tt.right, tt.w, "…")
			if got != tt.want || ansi.StringWidth(got) != tt.w {
				t.Errorf("Spread = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPadLines(t *testing.T) {
	if got := PadLines([]string{"ab"}, 2, 3); !slices.Equal(got, []string{"ab", "  ", "  "}) {
		t.Errorf("padded to %q", got)
	}
	if got := PadLines([]string{"ab", "cd"}, 2, 1); !slices.Equal(got, []string{"ab"}) {
		t.Errorf("cut to %q", got)
	}
	if got := PadLines(nil, 2, -1); len(got) != 0 {
		t.Errorf("negative height gave %q", got)
	}
}
