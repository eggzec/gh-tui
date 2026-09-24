package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// newSearchApp returns an app of fake sections with a dashboard and a
// search page, on an 80x24 terminal, opened on repo if it isn't zero. The
// search page is the last fake.
func newSearchApp(t *testing.T, repo core.RepoRef) (*Model, []*fakeSection) {
	t.Helper()
	fakes := []*fakeSection{
		{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"},
		{title: ui.DashboardTitle}, {title: ui.SearchTitle},
	}
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3], Dashboard: fakes[4], Search: fakes[5]}
	var opts []Option
	if repo != (core.RepoRef{}) {
		opts = append(opts, WithRepo(repo))
	}
	m := New(t.Context(), config.Default(), layout, opts...)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(m, m.Init())
	return m, fakes
}

func TestSearchKeyShowsThePage(t *testing.T) {
	for _, start := range []core.RepoRef{testRepo, {}} {
		m, fakes := newSearchApp(t, start)
		was := m.screen
		run(m, m.key(press("/")))
		if m.screen != searchScreen || !fakes[5].focused || fakes[5].inits != 1 {
			t.Fatalf("/ didn't show the search page: screen %d, focused %v", m.screen, focusedTitles(fakes))
		}
		if d := fakes[5]; d.width != 80 || d.height != 22 {
			t.Errorf("the page is %dx%d, want the whole screen of 80x22", d.width, d.height)
		}
		if s := onScreen(m); !strings.Contains(s, "─ Search ─") {
			t.Errorf("the header should name the page:\n%s", s)
		}
		run(m, func() tea.Msg { return ui.BackMsg{} })
		if m.screen != was {
			t.Errorf("BackMsg went to screen %d, want %d", m.screen, was)
		}
	}
}

func TestSearchKeyOnThePageFocusesTheQuery(t *testing.T) {
	m, fakes := newSearchApp(t, testRepo)
	run(m, m.key(press("/")))
	fakes[5].focused = false
	run(m, m.key(press("/")))
	if m.screen != searchScreen || !fakes[5].focused {
		t.Error("/ on the page should focus it again")
	}
}

func TestSearchPageCapturesKeys(t *testing.T) {
	m, fakes := newSearchApp(t, testRepo)
	run(m, m.key(press("/")))
	fakes[5].capturing = true
	for _, k := range []string{"q", "n", "0", "tab"} {
		if cmd := m.key(press(k)); cmd != nil {
			t.Errorf("%s returned a command of the app", k)
		}
		if m.screen != searchScreen || !fakes[5].got(isKey(k)) {
			t.Errorf("%s should reach the page while it captures", k)
		}
	}
}

func TestRepoFromSearch(t *testing.T) {
	m, fakes := newSearchApp(t, core.RepoRef{})
	run(m, m.key(press("/")))
	run(m, func() tea.Msg { return ui.RepoMsg{Repo: testRepo} })
	if m.screen != repoScreen || !fakes[0].focused {
		t.Fatalf("a repository chosen in the search should show its files: screen %d", m.screen)
	}
	run(m, m.key(press("/")))
	if m.screen != searchScreen {
		t.Error("/ should bring the search back")
	}
	m.Update(ui.ShowMsg{Title: ui.DashboardTitle})
	m.Update(ui.ShowMsg{Title: ui.SearchTitle})
	if m.screen != searchScreen {
		t.Error("ShowMsg didn't show the search page")
	}
}
