package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// newHeaderApp returns an app of fake sections with a dashboard, opened
// on testRepo, on its main branch, with three unread notifications.
func newHeaderApp(t *testing.T, opts ...Option) *Model {
	t.Helper()
	layout := Layout{
		Files: &fakeSection{title: "Files"}, Pulls: &fakeSection{title: "Pull requests"},
		Notifications: &fakeSection{title: ui.NotificationsTitle, badge: "3"},
		Dashboard:     &fakeSection{title: ui.DashboardTitle},
	}
	m := New(t.Context(), config.Default(), layout, append([]Option{WithRepo(testRepo)}, opts...)...)
	rankFakes(m)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(repoInfoMsg{repo: core.Repo{Ref: testRepo, DefaultBranch: "main"}})
	m.Update(ui.SyncMsg{})
	return m
}

// TestHeaderFollowsTheScreen goes from the repository to the dashboard and
// back, and checks that each shows its own title, and that only the
// repository screen names the repository.
func TestHeaderFollowsTheScreen(t *testing.T) {
	t.Parallel()
	m := newHeaderApp(t)
	steps := []struct {
		key, want string
		repo      bool
	}{
		{"", testRepo.String() + " ─ main", true},
		{"0", ui.DashboardTitle, false},
		{"backspace", testRepo.String() + " ─ main", true},
	}
	for _, s := range steps {
		if s.key != "" {
			run(m, m.key(press(s.key)))
		}
		h := ansi.Strip(m.header)
		if !strings.HasPrefix(h, "─ "+s.want+" ") {
			t.Errorf("after %q the header is %q, want it to start with %q", s.key, h, s.want)
		}
		if got := strings.Contains(h, testRepo.String()) || strings.Contains(h, "main"); got != s.repo {
			t.Errorf("after %q the header names the repository or branch: %t, want %t: %q", s.key, got, s.repo, h)
		}
	}
}
