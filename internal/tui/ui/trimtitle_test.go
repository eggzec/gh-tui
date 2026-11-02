package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// A long title is cut with an ellipsis, the name stays, and the glyph is
// dropped before the name is cut.
func TestTrimTitleFit(t *testing.T) {
	t.Parallel()
	const glyph = "\x1b[31m*\x1b[m"
	long := strings.Repeat("long title ", 12)
	for _, tt := range []struct {
		name  string
		width int
		parts TrimTitle
		want  string
	}{
		{"name only", 20, TrimTitle{Name: "#7"}, "#7"},
		{"loaded", 40, TrimTitle{Lead: glyph, Name: "#7", Detail: "Fix it"}, "* #7 Fix it"},
		{"detail cut first", 20, TrimTitle{Lead: glyph, Name: "#7", Detail: long}, "* #7 long title lon…"},
		{"glyph dropped", 3, TrimTitle{Lead: glyph, Name: "#7", Detail: long}, "#7"},
		{"name cut", 5, TrimTitle{Name: "a long name"}, "a lo…"},
		{"newline", 20, TrimTitle{Name: "a\nb"}, "a b"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.parts.Fit(tt.width, lipgloss.NewStyle(), "…")
			if plain := ansi.Strip(got); plain != tt.want {
				t.Errorf("Fit = %q, want %q", plain, tt.want)
			}
			if w := lipgloss.Width(got); w > tt.width {
				t.Errorf("Fit is %d cells wide, in %d", w, tt.width)
			}
		})
	}
}
