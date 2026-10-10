package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	searchpage "github.com/eggzec/gh-tui/internal/tui/search"
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
	t.Parallel()
	for _, start := range []core.RepoRef{testRepo, {}} {
		m, fakes := newSearchApp(t, start)
		was := m.screen
		run(m, m.key(press("S")))
		if m.screen != searchScreen || !fakes[5].focused || fakes[5].inits != 1 {
			t.Fatalf("/ didn't show the search page: screen %d, focused %v", m.screen, focusedTitles(fakes))
		}
		if d := fakes[5]; d.width != 80 || d.height != 22 {
			t.Errorf("the page is %dx%d, want the whole screen of 80x22", d.width, d.height)
		}
		if s := onScreen(m); !strings.Contains(s, "─ Search ─") {
			t.Errorf("the header should name the page:\n%s", s)
		}
		// The query types backspace, so leave it first.
		run(m, m.key(press("esc")))
		run(m, m.key(press("backspace")))
		if m.screen != was {
			t.Errorf("backspace went to screen %d, want %d", m.screen, was)
		}
	}
}

func TestSearchKeyOnThePageFocusesTheQuery(t *testing.T) {
	t.Parallel()
	m, fakes := newSearchApp(t, testRepo)
	run(m, m.key(press("S")))
	fakes[5].focused = false
	run(m, m.key(press("S")))
	if m.screen != searchScreen || !fakes[5].focused {
		t.Error("/ on the page should focus it again")
	}
}

func TestSearchPageCapturesKeys(t *testing.T) {
	t.Parallel()
	m, fakes := newSearchApp(t, testRepo)
	run(m, m.key(press("S")))
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
	t.Parallel()
	m, fakes := newSearchApp(t, core.RepoRef{})
	run(m, m.key(press("S")))
	run(m, func() tea.Msg { return ui.RepoMsg{Repo: testRepo} })
	if m.screen != repoScreen || !fakes[0].focused {
		t.Fatalf("a repository chosen in the search should show its files: screen %d", m.screen)
	}
	run(m, m.key(press("S")))
	if m.screen != searchScreen {
		t.Error("/ should bring the search back")
	}
	m.Update(ui.ShowMsg{Title: ui.DashboardTitle})
	m.Update(ui.ShowMsg{Title: ui.SearchTitle})
	if m.screen != searchScreen {
		t.Error("ShowMsg didn't show the search page")
	}
}

// TestPreviewFromSearch opens what the search found in the modals of the
// sections that own them: over the page, which stays on view with the
// focus, and comes back once the modal closes.
func TestPreviewFromSearch(t *testing.T) {
	t.Parallel()
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	for _, tt := range []struct {
		name  string
		owner int
		open  tea.Msg
	}{
		{"issue", 2, ui.OpenIssueMsg{Repo: other, Number: 1203, ShowRepo: true}},
		{"pull request", 1, ui.OpenPullMsg{Repo: other, Number: 1388, ShowRepo: true}},
		{"file", 0, ui.OpenFileMsg{Repo: other, Path: "tea.go", SHA: "b1", Find: "tea"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, fakes := newSearchApp(t, testRepo)
			mod := &fakeModal{title: tt.name}
			fakes[tt.owner].reply = func(msg tea.Msg) tea.Cmd {
				if msg == tt.open {
					return ui.OpenModal(mod)
				}
				return nil
			}
			run(m, m.key(press("S")))
			run(m, func() tea.Msg { return tt.open })
			if m.topModal() != mod || m.screen != searchScreen || m.repo != testRepo {
				t.Fatalf("modal %v on screen %d for %v, want the %s over the search, with the repository kept",
					m.topModal(), m.screen, m.repo, tt.name)
			}
			if !fakes[5].focused {
				t.Error("the search page should keep the focus under the modal")
			}
			fakes[5].msgs = nil
			run(m, m.key(press("j")))
			if fakes[5].got(isKey("j")) || !slices.Contains(mod.keys(), "j") {
				t.Error("the modal should take the keys while it is open")
			}
			m.refreshBar()
			if len(m.hints) != 2 || ansi.Strip(m.hints[0].Forms[0]) != "? help" || !strings.HasSuffix(ansi.Strip(m.hints[1].Forms[0]), "close") {
				t.Errorf("the status bar offers %d keys, want the help and the modal's alone", len(m.hints))
			}
			run(m, ui.CloseModal(mod))
			if m.topModal() != nil || m.screen != searchScreen || !fakes[5].focused {
				t.Errorf("after the modal closed: modal %v on screen %d, want the search page", m.topModal(), m.screen)
			}
			run(m, m.key(press("j")))
			if !fakes[5].got(isKey("j")) {
				t.Error("the page should take the keys again once the modal closed")
			}
		})
	}
}

// fakeSearch is a search page that records what it was asked to search
// for.
var (
	_ Searcher = (*searchpage.Section)(nil)
	_ Fresher  = (*searchpage.Section)(nil)
)

type fakeSearch struct {
	fakeSection
	queries []string
	fresh   int
	log     []string
}

func (f *fakeSearch) Fresh() { f.fresh++; f.log = append(f.log, "fresh") }

func (f *fakeSearch) Search(query string) tea.Cmd {
	f.queries = append(f.queries, query)
	f.log = append(f.log, "search")
	return nil
}

func TestSearchCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		line string
		want []string
	}{
		{line: "search", want: nil},
		{line: "search bubble tea", want: []string{"bubble tea"}},
		{line: "search   is:open   label:bug ", want: []string{"is:open   label:bug"}},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			page := &fakeSearch{title: ui.SearchTitle}
			fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}}
			m := New(t.Context(), config.Default(), Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Search: page}, WithRepo(testRepo))
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			runCommand(t, m, tt.line)
			if m.screen != searchScreen || !page.focused || page.inits != 1 {
				t.Fatalf("search didn't show the page as / does: screen %d, focused %v", m.screen, page.focused)
			}
			if !slices.Equal(page.queries, tt.want) {
				t.Errorf("searched for %q, want %q", page.queries, tt.want)
			}
			wantLog := []string{"fresh"}
			if len(tt.want) > 0 {
				wantLog = append(wantLog, "search")
			}
			if !slices.Equal(page.log, wantLog) {
				t.Errorf("the page was asked %q, want %q: reset first, then search", page.log, wantLog)
			}
		})
	}
	t.Run("no search page", func(t *testing.T) {
		m, _ := newTestApp(t)
		runCommand(t, m, "search tea")
		if m.screen != repoScreen || !hasToast(m, "There is no search page.") {
			t.Errorf("screen %d, toasts %q", m.screen, toasted(m))
		}
	})
}

func TestSearchKeyStartsFresh(t *testing.T) {
	t.Parallel()
	page := &fakeSearch{title: ui.SearchTitle}
	m := New(t.Context(), config.Default(), Layout{Files: &fakeSection{title: "Files"}, Search: page})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(m, m.Init())
	drive(m, m.key(press("S")))
	drive(m, m.key(press("S")))
	if page.fresh != 2 {
		t.Errorf("the search key made the page start fresh %d times in 2 presses, want 2", page.fresh)
	}
}
