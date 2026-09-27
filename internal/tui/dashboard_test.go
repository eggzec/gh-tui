package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// newDashApp returns an app of fake sections with a dashboard, on an 80x24
// terminal, opened on repo if it isn't zero. The dashboard is the last
// fake.
func newDashApp(t *testing.T, repo core.RepoRef) (*Model, []*fakeSection) {
	t.Helper()
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}, {title: ui.DashboardTitle}}
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3], Dashboard: fakes[4]}
	var opts []Option
	if repo != (core.RepoRef{}) {
		opts = append(opts, WithRepo(repo))
	}
	m := New(t.Context(), config.Default(), layout, opts...)
	m.toast.SetDuration(0)
	m.toast.SetErrorDuration(0)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(m, m.Init())
	return m, fakes
}

func TestStartScreen(t *testing.T) {
	tests := []struct {
		name  string
		repo  core.RepoRef
		want  screen
		focus string
	}{
		{"without a repository", core.RepoRef{}, dashScreen, ui.DashboardTitle},
		{"with a repository", testRepo, repoScreen, "Files"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, fakes := newDashApp(t, tt.repo)
			if m.screen != tt.want {
				t.Errorf("the app opened on screen %d, want %d", m.screen, tt.want)
			}
			if got := focusedTitles(fakes); !slices.Equal(got, []string{tt.focus}) {
				t.Errorf("focused = %v, want %s", got, tt.focus)
			}
			dashStarted := fakes[4].inits == 1
			if dashStarted != (tt.want == dashScreen) {
				t.Errorf("the dashboard started %d times", fakes[4].inits)
			}
		})
	}
}

func TestDashboardFillsTheScreen(t *testing.T) {
	m, fakes := newDashApp(t, core.RepoRef{})
	// Only the header and the help line are the app's.
	if d := fakes[4]; d.width != 80 || d.height != 22 {
		t.Errorf("the dashboard is %dx%d, want 80x22", d.width, d.height)
	}
	s := onScreen(m)
	if !strings.Contains(s, "─ Dashboard ─") || !strings.Contains(s, "Dashboard content") || strings.Contains(s, "╭") {
		t.Errorf("the dashboard should fill the screen unframed, under its header:\n%s", s)
	}
}

func TestDashboardKey(t *testing.T) {
	m, fakes := newDashApp(t, testRepo)
	run(m, m.key(press("2")))
	run(m, m.key(press("0")))
	if m.screen != dashScreen || !fakes[4].focused {
		t.Fatalf("0 didn't show the dashboard: focused %v", focusedTitles(fakes))
	}
	if s := onScreen(m); !strings.Contains(s, "0 back") {
		t.Errorf("the help should say 0 goes back:\n%s", s)
	}
	run(m, m.key(press("0")))
	if m.screen != repoScreen || !fakes[1].focused {
		t.Errorf("0 again should go back to the pane that had focus: %v", focusedTitles(fakes))
	}
	if s := onScreen(m); !strings.Contains(s, "eggzec/gh-tui") || !strings.Contains(s, "0 dashboard") {
		t.Errorf("the repository screen should be back, with the dashboard key in the help:\n%s", s)
	}
}

func TestDashboardTakesPaneKeys(t *testing.T) {
	m, fakes := newDashApp(t, core.RepoRef{})
	for _, k := range []string{"tab", "shift+tab", "1", "2", "3"} {
		run(m, m.key(press(k)))
		if m.screen != dashScreen {
			t.Fatalf("%s left the dashboard", k)
		}
		if !fakes[4].got(isKey(k)) {
			t.Errorf("the dashboard didn't get %s", k)
		}
	}
}

func TestNotificationsFromDashboard(t *testing.T) {
	m, fakes := newDashApp(t, core.RepoRef{})
	run(m, m.key(press("n")))
	if m.screen != notifScreen {
		t.Fatal("n didn't show the notifications")
	}
	run(m, m.key(press("n")))
	if m.screen != dashScreen || !fakes[4].focused {
		t.Error("n again should go back to the dashboard")
	}

	// The dashboard shows the notifications, and the app goes back to it.
	m.Update(ui.ShowMsg{Title: ui.NotificationsTitle})
	if m.screen != notifScreen {
		t.Fatal("ShowMsg didn't show the notifications")
	}
	run(m, m.key(press("n")))
	if m.screen != dashScreen {
		t.Error("n should go back to the dashboard that showed the notifications")
	}
	run(m, m.key(press("n")))
	m.Update(ui.ShowMsg{Title: ui.DashboardTitle})
	if m.screen != dashScreen {
		t.Error("ShowMsg didn't show the dashboard")
	}
}

func TestRepoFromDashboard(t *testing.T) {
	m, fakes := newDashApp(t, core.RepoRef{})
	run(m, func() tea.Msg { return ui.RepoMsg{Repo: testRepo} })
	if m.screen != repoScreen || !fakes[0].focused || fakes[0].inits != 1 {
		t.Errorf("a repository chosen on the dashboard should show its files: screen %d, focused %v", m.screen, focusedTitles(fakes))
	}
	if s := onScreen(m); !strings.Contains(s, "eggzec/gh-tui") {
		t.Errorf("the header should name the repository:\n%s", s)
	}
	// The dashboard is told too, as every section is.
	if !fakes[4].got(func(msg tea.Msg) bool { _, ok := msg.(ui.RepoMsg); return ok }) {
		t.Error("the dashboard wasn't told the repository")
	}
}

func TestNoDashboard(t *testing.T) {
	m, fakes := newApp(t, core.RepoRef{})
	run(m, m.key(press("0")))
	if m.screen != notifScreen || !slices.Equal(focusedTitles(fakes), []string{"Notifications"}) {
		t.Error("without a dashboard, 0 does nothing")
	}
	if s := onScreen(m); strings.Contains(s, "dashboard") {
		t.Errorf("without a dashboard its key isn't in the help:\n%s", s)
	}
}

func TestProgramOpensOnDashboard(t *testing.T) {
	dash := &fakeSection{title: ui.DashboardTitle}
	files := &fakeSection{title: "Files"}
	app := New(t.Context(), config.Default(), Layout{Files: files, Notifications: &fakeSection{title: "Notifications"}, Dashboard: dash})
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(80, 24))
	tm.Send(ui.RepoMsg{Repo: testRepo})
	tm.Send(press("0"))
	tm.Send(press("q"))
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(time.Second)).(*Model)
	if !ok || final.screen != dashScreen || final.back != repoScreen || final.repo != testRepo {
		t.Errorf("the final model should be on the dashboard, back from the repository")
	}
}

func BenchmarkViewDashboard(b *testing.B) {
	layout := Layout{
		Files: &fakeSection{title: "Files"}, Notifications: &fakeSection{title: "Notifications", badge: "3"},
		Dashboard: &fakeSection{title: ui.DashboardTitle},
	}
	m := New(b.Context(), config.Default(), layout)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}

// revisitSection is a fake section that counts the times the app brought
// it back from another screen.
type revisitSection struct {
	fakeSection
	revisits int
}

// revisitedMsg is what the command of a Revisit sends.
type revisitedMsg struct{}

func (s *revisitSection) Revisit() tea.Cmd {
	s.revisits++
	return func() tea.Msg { return revisitedMsg{} }
}

// Coming back to the dashboard from another screen revisits it, and runs
// what that returns; showing it the first time only starts it.
func TestDashboardRevisitedOnReturn(t *testing.T) {
	dash := &revisitSection{title: ui.DashboardTitle}
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}}
	m := New(t.Context(), config.Default(), Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3], Dashboard: dash})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(m, m.Init())
	if dash.inits != 1 || dash.revisits != 0 {
		t.Fatalf("opening on the dashboard: %d inits and %d revisits, want 1 and none", dash.inits, dash.revisits)
	}
	run(m, m.key(press("n")))
	run(m, m.key(press("n")))
	if m.screen != dashScreen || dash.revisits != 1 {
		t.Fatalf("back on the dashboard: %d revisits, want 1", dash.revisits)
	}
	if !dash.got(func(msg tea.Msg) bool { _, ok := msg.(revisitedMsg); return ok }) {
		t.Error("the command of the revisit didn't run")
	}
	// Keys on the dashboard don't revisit it.
	run(m, m.key(press("tab")))
	if dash.revisits != 1 {
		t.Errorf("%d revisits without leaving, want 1", dash.revisits)
	}
}
