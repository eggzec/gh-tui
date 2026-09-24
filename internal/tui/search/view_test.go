package search

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
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
