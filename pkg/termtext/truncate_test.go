package termtext

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTruncate(t *testing.T) {
	for _, tt := range []struct {
		s     string
		width int
		tail  string
		want  string
	}{
		{"abcdef", 10, "...", "abcdef"},
		{"abcdef", 6, "...", "abcdef"},
		{"abcdef", 5, "...", "ab..."},
		{"abcdef", 3, "...", "..."},
		// A tail wider than the cut is left out.
		{"abcdef", 2, "...", "ab"},
		{"abcdef", 0, "...", ""},
		{"abcdef", 4, "…", "abc…"},
		{"abcdef", 0, "…", ""},
		{"\x1b[1mabcdef\x1b[m", 5, "...", "\x1b[1mab...\x1b[m"},
	} {
		if got := Truncate(tt.s, tt.width, tt.tail); got != tt.want {
			t.Errorf("Truncate(%q, %d, %q) = %q, want %q", tt.s, tt.width, tt.tail, got, tt.want)
		}
	}
}

func TestTruncateLeft(t *testing.T) {
	for _, tt := range []struct {
		s     string
		width int
		tail  string
		want  string
	}{
		{"abcdef", 10, "...", "abcdef"},
		{"abcdef", 6, "...", "abcdef"},
		{"abcdef", 5, "...", "...ef"},
		{"abcdef", 3, "...", "def"},
		{"abcdef", 2, "...", "ef"},
		{"abcdef", 0, "...", ""},
		{"abcdef", 4, "…", "…def"},
	} {
		if got := TruncateLeft(tt.s, tt.width, tt.tail); got != tt.want {
			t.Errorf("TruncateLeft(%q, %d, %q) = %q, want %q", tt.s, tt.width, tt.tail, got, tt.want)
		}
	}
}

func TestFit(t *testing.T) {
	for _, tt := range []struct {
		name  string
		s     string
		width int
		tail  string
		want  string
	}{
		{"plain pads", "abc", 6, "…", "abc   "},
		{"plain fits", "abcdef", 6, "…", "abcdef"},
		{"plain cut", "abcdef", 4, "…", "abc…"},
		{"styled boundary", "\x1b[1mabcdef\x1b[m", 5, "...", "\x1b[1mab...\x1b[m"},
		{"styled pads", "\x1b[1mab\x1b[m", 4, "...", "\x1b[1mab\x1b[m  "},
		{"wide rune at the cut", "ab世界", 4, "", "ab世"},
		{"wide rune cut leaves a gap", "ab世界", 3, "", "ab "},
		{"tail wider than width", "abcdef", 2, "...", "ab"},
		{"width 0", "abc", 0, "…", ""},
		{"negative width", "abc", -3, "…", ""},
	} {
		if got := Fit(tt.s, tt.width, tt.tail); got != tt.want {
			t.Errorf("%s: Fit(%q, %d, %q) = %q, want %q", tt.name, tt.s, tt.width, tt.tail, got, tt.want)
		}
	}
}

func TestTruncateMiddle(t *testing.T) {
	for _, tt := range []struct {
		s     string
		width int
		tail  string
		want  string
	}{
		{"abcdefghij", 10, "…", "abcdefghij"},
		{"abcdefghij", 20, "…", "abcdefghij"},
		{"abcdefghij", 7, "…", "abc…hij"},
		{"abcdefghij", 6, "…", "abc…ij"},
		{"abcdefghij", 8, "...", "abc...ij"},
		{"abcdefghij", 3, "...", "..."},
		// A tail wider than the cut is left out.
		{"abcdefghij", 2, "...", "ab"},
		{"abcdefghij", 1, "…", "…"},
		{"abcdefghij", 0, "…", ""},
		// A wide rune that straddles the cut is dropped.
		{"日本語日本語", 7, "…", "日…語"},
		{"日本語日本語", 12, "…", "日本語日本語"},
		{"\x1b[1mabcdefghij\x1b[m", 7, "…", "\x1b[1mabc\x1b[m…\x1b[1mhij\x1b[m"},
		// A wide rune that straddles only the right cut is dropped too.
		{"abcd日本x", 6, "…", "abc…x"},
		{"abcd🚀xyz", 6, "…", "abc…yz"},
	} {
		if got := TruncateMiddle(tt.s, tt.width, tt.tail); got != tt.want {
			t.Errorf("TruncateMiddle(%q, %d, %q) = %q, want %q", tt.s, tt.width, tt.tail, got, tt.want)
		}
	}
}

func TestTruncateMiddleFits(t *testing.T) {
	for _, s := range []string{"abcd日本x", "a日b本c語d", "🚀 ab🚀cd🚀 ef", "日本語日本語", "abcdefghij"} {
		for _, tail := range []string{"…", "...", "日"} {
			for w := range ansi.StringWidth(s) + 2 {
				if got := TruncateMiddle(s, w, tail); ansi.StringWidth(got) > w {
					t.Errorf("TruncateMiddle(%q, %d, %q) = %q, %d cells wide", s, w, tail, got, ansi.StringWidth(got))
				}
			}
		}
	}
}

func TestCells(t *testing.T) {
	for _, tt := range []struct {
		g    string
		n    int
		want string
	}{
		{">", 1, ">"},
		{"▌", 1, "▌"},
		{"->", 1, "-"},
		{"", 1, " "},
		{"x", 2, "x "},
		{"界", 1, " "},
	} {
		if got := Cells(tt.g, tt.n); got != tt.want {
			t.Errorf("Cells(%q, %d) = %q, want %q", tt.g, tt.n, got, tt.want)
		}
	}
}
