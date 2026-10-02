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
