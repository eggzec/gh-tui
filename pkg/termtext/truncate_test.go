package termtext

import "testing"

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
		{"日本語日本語", 7, "…", "日…本語"},
		{"日本語日本語", 12, "…", "日本語日本語"},
		{"\x1b[1mabcdefghij\x1b[m", 7, "…", "\x1b[1mabc\x1b[m…\x1b[1mhij\x1b[m"},
	} {
		if got := TruncateMiddle(tt.s, tt.width, tt.tail); got != tt.want {
			t.Errorf("TruncateMiddle(%q, %d, %q) = %q, want %q", tt.s, tt.width, tt.tail, got, tt.want)
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
