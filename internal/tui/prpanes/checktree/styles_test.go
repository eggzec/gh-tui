package checktree

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
)

// TestNamesKeepTheirStyle checks that the style of a name and of a name that
// folds reaches the row, over a link: the link cleans the text it is given.
func TestNamesKeepTheirStyle(t *testing.T) {
	t.Parallel()
	s := styles(config.IconsASCII)
	s.name = lipgloss.NewStyle().Bold(true)
	s.foldName = lipgloss.NewStyle().Italic(true)
	const addr = "https://github.com/o/r/actions/runs/1"
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"leaf", s.leaf(addr, "build\x1b[31m"), s.name.Render("build")},
		{"fold", s.fold(addr, "lint"), s.foldName.Render("lint")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !strings.HasPrefix(tt.got, "\x1b[") || !strings.Contains(tt.got, "\x1b[1m") && !strings.Contains(tt.got, "\x1b[3m") {
				t.Errorf("row %q has no style of its name", tt.got)
			}
			if got, want := ansi.Strip(tt.got), ansi.Strip(tt.want); got != want {
				t.Errorf("plain text = %q, want %q", got, want)
			}
		})
	}
}
