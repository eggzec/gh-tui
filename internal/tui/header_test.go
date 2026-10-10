package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

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
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(repoInfoMsg{repo: core.Repo{Ref: testRepo, DefaultBranch: "main"}})
	m.Update(ui.SyncMsg{})
	return m
}

func TestNotificationsHeader(t *testing.T) {
	t.Parallel()
	for _, width := range []int{40, 80, 120} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			m := newHeaderApp(t)
			run(m, m.key(press("I")))
			m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			if got := ansi.StringWidth(m.header); got != width {
				t.Errorf("header is %d wide, want %d", got, width)
			}
			golden.RequireEqual(t, m.header)
		})
	}
}

// TestHeaderFollowsTheScreen goes from the repository to the notifications,
// the dashboard and the notifications again, and checks that each shows its
// own title, and that only the repository screen names the repository.
func TestHeaderFollowsTheScreen(t *testing.T) {
	t.Parallel()
	m := newHeaderApp(t)
	steps := []struct {
		key, want string
		repo      bool
	}{
		{"", testRepo.String() + " ─ main", true},
		{"I", ui.NotificationsTitle, false},
		{"0", ui.DashboardTitle, false},
		{"I", ui.NotificationsTitle, false},
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
