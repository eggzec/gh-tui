package search

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// app hosts the page as the program root, the way the tui would, and
// records what the page sends it.
type app struct {
	s    *Section
	sent []tea.Msg
	// got hears of every message sent, so the test knows when to quit,
	// and ready is closed once the results of the entered query are
	// listed, with the cursor on one.
	got   chan tea.Msg
	ready chan struct{}
	shown bool
}

func (a *app) Init() tea.Cmd { return a.s.Init() }

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.s.SetSize(msg.Width, msg.Height)
		return a, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	case ui.RepoMsg, ui.OpenPullMsg, ui.OpenIssueMsg, ui.OpenFileMsg, ui.OpenMsg:
		a.sent = append(a.sent, msg)
		a.got <- msg
		return a, nil
	}
	cmd := a.s.Update(msg)
	// The results of a query typed so far may come while typing, so
	// only those shown after enter, which focuses them, will do.
	if a.s.area == resultsArea && !a.shown {
		if l, ok := a.s.visibleHits(); ok {
			if _, ok := l.feed.Selected(); ok {
				a.shown = true
				close(a.ready)
			}
		}
	}
	return a, cmd
}

func (a *app) View() tea.View { return tea.NewView(a.s.View()) }

func TestProgram(t *testing.T) {
	svc := newFake()
	s := New(t.Context(), svc, config.Default().Keys, WithNow(func() time.Time { return now }), WithDebounce(0))
	s.Focus()
	a := &app{s: s, got: make(chan tea.Msg, 8), ready: make(chan struct{})}
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(120, 30))
	wait := func(ch <-chan struct{}, what string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal(what)
		}
	}
	sent := func() <-chan struct{} {
		done := make(chan struct{})
		go func() { <-a.got; close(done) }()
		return done
	}

	// Type a query and press enter, which searches at once and focuses the
	// results, then open the first once it is listed, and go back.
	for _, r := range "bubbletea" {
		tm.Send(keyPress(string(r)))
	}
	tm.Send(keyPress("enter"))
	wait(a.ready, "the page listed no results")
	tm.Send(keyPress("enter"))
	wait(sent(), "the page didn't open the repository")
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*app)
	want := []tea.Msg{ui.RepoMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}}}
	if !slices.Equal(final.sent, want) {
		t.Errorf("the page sent %v, want %v", final.sent, want)
	}
	if final.s.Query() != "bubbletea" || final.s.recent[0] != "bubbletea" {
		t.Errorf("the page searched %q and remembered %v", final.s.Query(), final.s.recent)
	}
}
