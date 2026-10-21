package tui

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	searchpage "github.com/eggzec/gh-tui/internal/tui/search"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// newSearchApp returns an app of fake sections with a dashboard and a real
// search section, on an 80x24 terminal, opened on repo if it isn't zero. The
// search shows in a modal, so it needs the section itself.
func newSearchApp(t *testing.T, repo core.RepoRef) (*Model, []*fakeSection) {
	t.Helper()
	cfg := config.Default()
	fakes := []*fakeSection{
		{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"},
		{title: ui.DashboardTitle},
	}
	layout := Layout{
		Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3], Dashboard: fakes[4],
		Search: searchpage.New(t.Context(), keySearch{}, cfg.Keys),
	}
	var opts []Option
	if repo != (core.RepoRef{}) {
		opts = append(opts, WithRepo(repo))
	}
	m := New(t.Context(), cfg, layout, opts...)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(m, m.Init())
	return m, fakes
}

func TestSearchKeyShowsTheModal(t *testing.T) {
	t.Parallel()
	for _, start := range []core.RepoRef{testRepo, {}} {
		m, _ := newSearchApp(t, start)
		was := m.screen
		run(m, m.key(press("S")))
		if m.searchHost() == nil || m.screen != was {
			t.Fatalf("S didn't show the search in a modal: modal %v, screen %d, want %d", m.modal, m.screen, was)
		}
		// The query types backspace, so leave it first.
		run(m, m.key(press("esc")))
		run(m, m.key(press("backspace")))
		if m.searchHost() != nil || m.screen != was {
			t.Errorf("backspace left modal %v on screen %d, want the search closed on %d", m.modal, m.screen, was)
		}
	}
}

func TestSearchKeyInTheModalFocusesTheQuery(t *testing.T) {
	t.Parallel()
	m, _ := newSearchApp(t, testRepo)
	run(m, m.key(press("S")))
	run(m, m.key(press("i")))
	if got := focusOf(m); got != `search: typing ""` {
		t.Errorf("S in the modal left the focus at %s, want the query typing", got)
	}
}

func TestSearchModalCapturesKeys(t *testing.T) {
	t.Parallel()
	m, _ := newSearchApp(t, testRepo)
	run(m, m.key(press("S")))
	run(m, m.key(press("i")))
	for _, k := range []string{"q", "n", "0"} {
		drive(m, m.key(press(k)))
		if m.searchHost() == nil {
			t.Fatalf("%s closed the search while the query types", k)
		}
	}
	if got := m.srch.section.(*searchpage.Section).Query(); got != "qn0" {
		t.Errorf("the query is %q, want the typed keys", got)
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
	if m.modal != nil {
		t.Errorf("the search stays open over the repository: %v", m.modal)
	}
	run(m, m.key(press("S")))
	if m.searchHost() == nil {
		t.Error("S should bring the search back")
	}
}

// TestPreviewFromSearch opens what the search found in the modals of the
// sections that own them: over the search, which comes back once the modal
// closes.
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
				// The root sets the modal that the new one replaces before
				// the section sees the message.
				switch o := msg.(type) {
				case ui.OpenPullMsg:
					o.Back = nil
					msg = o
				case ui.OpenIssueMsg:
					o.Back = nil
					msg = o
				}
				if msg == tt.open {
					return ui.OpenModal(mod)
				}
				return nil
			}
			run(m, m.key(press("S")))
			search := m.modal
			run(m, func() tea.Msg { return tt.open })
			if m.topModal() != mod || m.screen != repoScreen || m.repo != testRepo {
				t.Fatalf("modal %v on screen %d for %v, want the %s over the search, with the repository kept",
					m.topModal(), m.screen, m.repo, tt.name)
			}
			run(m, m.key(press("j")))
			if !slices.Contains(mod.keys(), "j") {
				t.Error("the modal should take the keys while it is open")
			}
			m.refreshBar()
			if len(m.hints) != 2 || ansi.Strip(m.hints[0].Forms[0]) != "? help" {
				t.Errorf("the status bar offers %d keys, want the help and the modal's alone", len(m.hints))
			}
			run(m, ui.CloseModal(mod))
			if m.topModal() != search || m.searchHost() == nil {
				t.Fatalf("modal %v after the %s closed, want the search back", m.topModal(), tt.name)
			}
			run(m, m.key(press("i")))
			if got := focusOf(m); got != `search: typing ""` {
				t.Errorf("the search came back with the focus at %s, want the query typing", got)
			}
		})
	}
}

var (
	_ Searcher = (*searchpage.Section)(nil)
	_ Fresher  = (*searchpage.Section)(nil)
)

// screenModal is a fake modal in a registered, non-capturing context, so
// that the command line opens over it as it does over a real modal.
type screenModal struct{ *fakeModal }

func (s *screenModal) KeyLayers() []keyhelp.Layer {
	l := s.fakeModal.KeyLayers()
	l[0].Context = "history"
	return l
}

func TestSearchCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		line string
		want string
	}{
		{line: "search", want: ""},
		{line: "search bubble tea", want: "bubble tea"},
		{line: "search   is:open   label:bug ", want: "is:open label:bug"},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			m, _ := newSearchApp(t, testRepo)
			runCommand(t, m, tt.line)
			if m.searchHost() == nil {
				t.Fatalf("search didn't show the modal as S does: modal %v", m.modal)
			}
			if got := m.srch.section.(*searchpage.Section).Query(); got != tt.want {
				t.Errorf("searched for %q, want %q", got, tt.want)
			}
		})
	}
	t.Run("no search", func(t *testing.T) {
		m, _ := newTestApp(t)
		runCommand(t, m, "search tea")
		if m.screen != repoScreen || !hasToast(m, "There is no search.") {
			t.Errorf("screen %d, toasts %q", m.screen, toasted(m))
		}
	})
	t.Run("over a modal", func(t *testing.T) {
		m, _ := newSearchApp(t, testRepo)
		mod := &screenModal{&fakeModal{title: "other"}}
		m.openModal(mod)
		runCommand(t, m, "search foo")
		if m.isOpen(mod) || m.searchHost() == nil {
			t.Fatalf("search left modal %v open, want the modal closed and the search shown", m.modal)
		}
		if got := m.srch.section.(*searchpage.Section).Query(); got != "foo" {
			t.Errorf("searched for %q, want foo", got)
		}
	})
	t.Run("inside the search", func(t *testing.T) {
		m, _ := newSearchApp(t, testRepo)
		run(m, m.key(press("S")))
		runCommand(t, m, "search bar")
		if m.searchHost() == nil {
			t.Fatalf("search closed the search: modal %v", m.modal)
		}
		if got := m.srch.section.(*searchpage.Section).Query(); got != "bar" {
			t.Errorf("searched for %q, want bar", got)
		}
	})
}

func TestSearchKeyStartsFresh(t *testing.T) {
	t.Parallel()
	m, _ := newSearchApp(t, testRepo)
	drive(m, m.key(press("S")))
	for _, k := range []string{"a", "b"} {
		drive(m, m.key(press(k)))
	}
	drive(m, m.key(press("esc")))
	drive(m, m.key(press("S")))
	if got := m.srch.section.(*searchpage.Section).Query(); got != "" {
		t.Errorf("the search key left the query %q, want it empty", got)
	}
}
