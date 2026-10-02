package uitest

import (
	"fmt"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termimg"
)

// recorder keeps the errors of a check instead of failing.
type recorder struct {
	testing.TB
	errs []string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

// The guard tells cells that keep what names their image from cells that
// a style or a cut has spoiled.
func TestPlaceholders(t *testing.T) {
	row := termimg.Rows(termimg.NewID(7, 42), 3, 1)[0]
	tests := []struct {
		name  string
		view  string
		cells int
		bad   bool
	}{
		{"plain", "ab " + row + " cd", 3, false},
		{"cut", ansi.Truncate("ab "+row, 4, ""), 1, false},
		{"inside a style", lipgloss.NewStyle().Bold(true).Render("x") + row, 3, false},
		{"reversed", lipgloss.NewStyle().Reverse(true).Render(row), 3, true},
		{"recolored", lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Render(ansi.Strip(row)), 3, true},
		{"no image", "plain text", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{TB: t}
			if n := Placeholders(r, tt.view); n != tt.cells {
				t.Errorf("%d cells, want %d", n, tt.cells)
			}
			if bad := len(r.errs) > 0; bad != tt.bad {
				t.Errorf("errors %q, want some: %v", r.errs, tt.bad)
			}
		})
	}
}
