package dashboard

import (
	"cmp"
	"fmt"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// app hosts the dashboard as the program root, the way the tui would, and
// records the messages it sends the app.
type app struct {
	s    *Section
	sent []tea.Msg
	// got hears of every message sent, so the test knows when to quit.
	got chan tea.Msg
	// filtered hears once the list shows the one repository the filter
	// keeps, so the test knows when to choose it.
	filtered chan struct{}
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
	case ui.RepoMsg, ui.OpenPullMsg, ui.OpenIssueMsg, ui.ShowMsg, ui.OpenMsg:
		a.sent = append(a.sent, msg)
		a.got <- msg
		return a, nil
	case filterform.AppliedMsg:
		// The app applies what its filter modal sends.
		return a, a.s.ApplyFilter(msg)
	}
	cmd := a.s.Update(msg)
	if o := a.s.repos.current(); a.filtered != nil && a.s.repos.filter().active() && o.feed.Settled() && o.feed.Len() == 1 {
		close(a.filtered)
		a.filtered = nil
	}
	return a, cmd
}

func (a *app) View() tea.View { return tea.NewView(a.s.View()) }

func TestProgram(t *testing.T) {
	// The dashboard paints from the cache, which holds the repositories of
	// github too, so the keys don't wait on any read.
	svc := newFake()
	svc.cached = true
	svc.read["github@"] = true
	s := New(t.Context(), svc, config.Default().Keys,
		WithNow(func() time.Time { return now }), WithHere(here, nil), WithInbox(&fakeInbox{threads: inboxThreads()}))
	s.Focus()
	filtered := make(chan struct{})
	a := &app{s: s, got: make(chan tea.Msg, 8), filtered: filtered}
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(140, 38))
	// Open a review request, filter the repositories of github by name and
	// open the one left, then open the repository here.
	for _, k := range []string{"3", "enter", "2", "]"} {
		tm.Send(keyPress(k))
	}
	tm.Send(filterform.AppliedMsg{Query: "r4"})
	select {
	case <-filtered:
	case <-time.After(5 * time.Second):
		t.Fatal("the filter didn't list one repository")
	}
	for _, k := range []string{"enter", "."} {
		tm.Send(keyPress(k))
	}
	// The messages come from commands, so quit once the last has arrived.
	for range 3 {
		select {
		case <-a.got:
		case <-time.After(5 * time.Second):
			t.Fatal("the dashboard didn't send three messages")
		}
	}
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*app)
	want := []tea.Msg{
		ui.OpenPullMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}, Number: 1402},
		ui.RepoMsg{Repo: core.RepoRef{Owner: "github", Name: "repo-004"}},
		ui.RepoMsg{Repo: here},
	}
	// The commands run at once, so the messages may arrive in any order.
	sent := slices.SortedFunc(slices.Values(final.sent), byString)
	if !slices.Equal(sent, slices.SortedFunc(slices.Values(want), byString)) {
		t.Errorf("the dashboard sent %v, want %v", final.sent, want)
	}
	if final.s.repos.current().label != "github" || final.s.repos.filter().query != "r4" {
		t.Error("the list should stay filtered, on the tab of github")
	}
}

func byString(a, b tea.Msg) int { return cmp.Compare(fmt.Sprint(a), fmt.Sprint(b)) }
