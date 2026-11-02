package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSeal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		line  string
		width int
		want  string
	}{
		{"pads", "ab", 4, "ab  \x1b[m"},
		{"cuts", "abcdef", 3, "abc\x1b[m"},
		{"exact", "abc", 3, "abc\x1b[m"},
		{"empty", "", 2, "  \x1b[m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Seal(tt.line, tt.width); got != tt.want {
				t.Errorf("Seal(%q, %d) = %q, want %q", tt.line, tt.width, got, tt.want)
			}
		})
	}
}

func TestChipRow(t *testing.T) {
	t.Parallel()
	chips := []string{"aaa", "bb", "cccc"}
	widths := []int{3, 2, 4}
	more := func(s string) string { return "\x1b[1m" + s + "\x1b[22m" }
	tests := []struct {
		name  string
		chips []string
		w     []int
		width int
		want  string
	}{
		{"all fit", chips, widths, 12, "aaa bb cccc \x1b[m"},
		{"drops the last", chips, widths, 10, "aaa bb \x1b[1m+1\x1b[22m \x1b[m"},
		{"keeps one", chips, widths, 6, "aaa \x1b[1m+2\x1b[22m\x1b[m"},
		{"only the count", chips, widths, 2, "\x1b[1m+3\x1b[22m\x1b[m"},
		{"no chips", nil, nil, 3, "   \x1b[m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ChipRow(tt.chips, tt.w, tt.width, more)
			if ansi.StringWidth(got) != tt.width {
				t.Errorf("width = %d, want %d: %q", ansi.StringWidth(got), tt.width, got)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
