package errline

import (
	"slices"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestLines(t *testing.T) {
	st := Styles{Mark: "✗", Separator: " · ", Ellipsis: "…"}
	tests := []struct {
		name       string
		text, hint string
		width      int
		rows       int
		want       []string
	}{
		{"hint after the text", "Can't reach GitHub", "r to retry", 40, 2, []string{"✗ Can't reach GitHub · r to retry"}},
		{"hint on its own line", "Can't reach GitHub", "r to retry", 24, 2, []string{"✗ Can't reach GitHub", "  r to retry"}},
		{"text cut on its last row", "one two three four five six seven", "", 16, 2, []string{"✗ one two three", "  four five six…"}},
		{"hint loses the indent rather than wrap", "Can't reach GitHub", "o to open on GitHub", 20, 1, []string{"✗ Can't reach GitHub", "o to open on GitHub"}},
		{"hint wider than the width wraps", "x", "o to open on GitHub", 14, 1, []string{"✗ x", "  o to open on", "  GitHub"}},
		{"no text leaves the hint", "", "r to retry", 20, 2, []string{"  r to retry"}},
		{"too narrow for the mark", "ab", "", 2, 2, []string{"ab"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Lines(st, tt.text, tt.hint, tt.width, tt.rows)
			for i := range got {
				got[i] = ansi.Strip(got[i])
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Lines = %q, want %q", got, tt.want)
			}
			for _, l := range got {
				if w := ansi.StringWidth(l); w > tt.width {
					t.Errorf("line %q is %d cells, over %d", l, w, tt.width)
				}
			}
		})
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  []string
	}{
		{"feat/some-branch name", 16, []string{"feat/some-branch", "name"}},
		{"abcdefgh", 3, []string{"abc", "def", "gh"}},
		{"界", 1, []string{"…"}},
		{"", 5, nil},
	}
	for _, tt := range tests {
		if got := Wrap(tt.s, tt.width); !slices.Equal(got, tt.want) {
			t.Errorf("Wrap(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}
