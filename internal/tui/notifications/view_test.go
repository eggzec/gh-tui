package notifications

import (
	"maps"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
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

// A row names the number of its subject, and the titles head the columns with
// the age, which is left-aligned, last.
func TestRefAndHeader(t *testing.T) {
	t.Parallel()
	r := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	if got, want := refLabel(r, 42, 40, "…"), "charmbracelet/bubbletea#42"; got != want {
		t.Errorf("refLabel = %q, want %q", got, want)
	}
	if got := refLabel(r, 42, 16, "…"); !strings.HasSuffix(got, "#42") || ansi.StringWidth(got) > 16 {
		t.Errorf("refLabel narrow = %q, want #42 kept within 16 cells", got)
	}
	if got := refLabel(r, 0, 40, "…"); got != "charmbracelet/bubbletea" {
		t.Errorf("refLabel without number = %q", got)
	}
	h := columnHeader(newLayout(100, 7, tagWidth), "…")
	if !strings.HasSuffix(strings.TrimRight(h, " "), "Updated") {
		t.Errorf("header = %q, want Updated last", h)
	}
	if narrow := columnHeader(newLayout(40, 4, 0), "…"); strings.Contains(narrow, "Reason") {
		t.Errorf("narrow header = %q, want the reason dropped", narrow)
	}
}

// An empty list names no key that isn't bound.
func TestEmptyTextWithoutKeys(t *testing.T) {
	keys := maps.Clone(config.Default().Keys)
	keys.Set("notifications.filter", nil)
	s := New(t.Context(), newFake(), keys)
	if got, want := s.emptyText(), "No unread notifications."; got != want {
		t.Errorf("empty text = %q, want %q", got, want)
	}
}
