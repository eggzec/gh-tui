package search

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		text          string
		keys          []string
	}{
		{"140 columns start", 140, 38, "", nil},
		{"140 columns repositories", 140, 38, "tea", []string{"down"}},
		{"140 columns issues", 140, 38, "tea", []string{"tab", "down"}},
		{"140 columns pull requests", 140, 38, "tea", []string{"tab", "down", "down", "enter"}},
		{"140 columns code", 140, 38, "tea", []string{"tab", "down", "down", "down", "enter"}},
		{"140 columns code on enter", 140, 38, "tea", []string{"tab", "down", "down", "down", "shift+tab", "s"}},
		{"140 columns invalid query", 140, 38, "tea is:bogus", nil},
		{"80 columns start", 80, 22, "", []string{"down"}},
		{"80 columns repositories", 80, 22, "tea", nil},
		{"80 columns issues", 80, 22, "tea", []string{"tab", "down", "enter"}},
		{"80 columns code", 80, 22, "tea", []string{"tab", "down", "down", "down", "enter"}},
		{"60 columns issues", 60, 22, "tea", []string{"tab", "down", "enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSection(t, newFake(), tt.width, tt.height)
			typeText(t, s, tt.text)
			press(t, s, tt.keys...)
			golden.RequireEqual(t, s.View())
		})
	}
}

// The repositories offered before the user types say why they failed, in
// both themes.
func TestViewStartFailed(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%t", dark), func(t *testing.T) {
			s := newSection(t, newFake(), 80, 22, WithIcons(ui.NewIcons(config.IconsUnicode)),
				WithStart(func(context.Context) ([]core.Repo, error) { return nil, errors.New("github: 502 Bad Gateway") }))
			p, err := config.Default().Palette(dark)
			if err != nil {
				t.Fatal(err)
			}
			s.SetTheme(ui.NewTheme(p, dark))
			golden.RequireEqual(t, s.View())
		})
	}
}
