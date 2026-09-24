package dashboard

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []string
	}{
		{"140 columns", 140, 38, nil},
		{"140 columns filter", 140, 38, []string{"f", "r", "e", "p", "o", "-", "1", "2"}},
		{"140 columns calendar", 140, 38, []string{"5", "left"}},
		{"80 columns", 80, 22, nil},
		{"80 columns pinned", 80, 22, []string{"1", "right"}},
		{"80 columns work", 80, 22, []string{"3", "down"}},
		{"80 columns notifications", 80, 22, []string{"4"}},
		{"80 columns calendar", 80, 22, []string{"5"}},
		{"80 columns filter", 80, 22, []string{"]", "f", "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSection(t, newFake(), &fakeInbox{threads: inboxThreads()}, tt.width, tt.height)
			press(t, s, tt.keys...)
			golden.RequireEqual(t, s.View())
		})
	}
}

func TestViewStates(t *testing.T) {
	t.Run("loading", func(t *testing.T) {
		s := New(t.Context(), newFake(), config.Default().Keys, WithNow(func() time.Time { return now }))
		s.SetSize(140, 38)
		s.Focus()
		golden.RequireEqual(t, s.View())
	})
	t.Run("empty", func(t *testing.T) {
		svc := newFake()
		svc.header.Pinned, svc.header.Orgs = nil, nil
		svc.work = core.Work{}
		svc.contrib = core.Contributions{}
		svc.repos = map[string][]core.Repo{}
		s := newSection(t, svc, &fakeInbox{}, 140, 38, WithHere(core.RepoRef{}, nil))
		golden.RequireEqual(t, s.View())
	})
}
