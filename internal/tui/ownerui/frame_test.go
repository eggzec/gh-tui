package ownerui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestDigits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		keys      []string
		cur       int
		want      string
		wantWidth int
	}{
		{"all bound", []string{"1", "2", "3"}, 1, "1 [2] 3", 7},
		{"unbound skipped", []string{"1", "", "3"}, 2, "1 [3]", 5},
		{"unbound first", []string{"", "2", "3"}, 2, "2 [3]", 5},
		{"focused unbound", []string{"1", "", "3"}, 1, "1 3", 3},
		{"all unbound", []string{"", "", ""}, 0, "", 0},
		{"no keys", nil, 0, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, w := Digits(tt.keys, tt.cur, Paint{}, Paint{})
			if got != tt.want || w != tt.wantWidth {
				t.Errorf("Digits = %q, %d; want %q, %d", got, w, tt.want, tt.wantWidth)
			}
			if strings.Contains(got, "  ") {
				t.Errorf("Digits %q has a double space", got)
			}
		})
	}
}

func TestFrameFoot(t *testing.T) {
	t.Parallel()
	const foot = "1 2 3"
	const footW = 5
	b := lipgloss.RoundedBorder()
	tests := []struct {
		name     string
		w        int
		footW    int
		wantFoot bool
	}{
		{"wide", 30, footW, true},
		{"fits exactly", footW + 5, footW, true},
		{"one too wide", footW + 4, footW, false},
		{"no foot", 30, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := ""
			if tt.footW > 0 {
				f = foot
			}
			lines := Frame("", 0, f, tt.footW, tt.w, 4, Paint{}, b, "", nil)
			if len(lines) != 4 {
				t.Fatalf("got %d lines, want 4", len(lines))
			}
			bottom := lines[len(lines)-1]
			if got := ansi.StringWidth(bottom); got != tt.w {
				t.Errorf("bottom line is %d cells wide, want %d: %q", got, tt.w, bottom)
			}
			if got := strings.Contains(bottom, foot); got != tt.wantFoot {
				t.Errorf("foot shown = %v, want %v: %q", got, tt.wantFoot, bottom)
			}
		})
	}
}

func TestFrameTiny(t *testing.T) {
	t.Parallel()
	b := lipgloss.RoundedBorder()
	for w := 2; w <= 5; w++ {
		t.Run(string(rune('0'+w)), func(t *testing.T) {
			t.Parallel()
			lines := Frame("label", 5, "1 2 3", 5, w, 4, Paint{}, b, "…", []string{"body"})
			if len(lines) != 4 {
				t.Fatalf("got %d lines, want 4", len(lines))
			}
			for i, l := range lines {
				if got := ansi.StringWidth(l); got != w {
					t.Errorf("line %d is %d cells wide, want %d: %q", i, got, w, l)
				}
			}
		})
	}
}
