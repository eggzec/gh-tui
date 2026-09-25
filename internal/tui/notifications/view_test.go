package notifications

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		threads       []core.Notification
		width, height int
		keys          []string
	}{
		{"unread at 80 columns", inbox(), 80, 8, nil},
		{"all at 80 columns", inbox(), 80, 9, []string{showAll, "down"}},
		{"all at 120 columns", inbox(), 120, 9, []string{showAll, "down"}},
		{"all at 50 columns", inbox(), 50, 9, []string{showAll}},
		{"empty", nil, 80, 4, nil},
		{"empty with all", nil, 80, 4, []string{showAll}},
		{"filtered at 80 columns", inbox(), 80, 6, []string{showAll + "reason:mention"}},
		{"filtered at 50 columns", inbox(), 50, 6, []string{showAll + "is:unread type:pr repo:charmbracelet/bubbletea"}},
		{"filtered empty", inbox(), 80, 4, []string{showAll + "type:release repo:cli/cli"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSection(t, newFake(tt.threads...), tt.width, tt.height)
			press(t, s, tt.keys...)
			golden.RequireEqual(t, s.View())
		})
	}
}
