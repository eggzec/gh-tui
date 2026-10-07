package history

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/graph"
)

// app hosts the modal as the program root, standing in for the tui: it
// keeps the base the modal sets, and ends once the modal closes.
type app struct {
	modal *Modal
	base  *ui.BaseMsg
	shown string
	// ready is closed once the graph selects its first commit.
	ready chan struct{}
}

func (a *app) Init() tea.Cmd { return a.modal.Init() }

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.modal.SetSize(msg.Width, msg.Height)
		return a, nil
	case ui.BaseMsg:
		a.base = &msg
		return a, nil
	case ui.ShowMsg:
		a.shown = msg.Title
		return a, tea.Quit
	case ui.CloseModalMsg:
		return a, nil
	case graph.SelectMsg:
		if a.ready != nil {
			close(a.ready)
			a.ready = nil
		}
	}
	return a, a.modal.Update(msg)
}

func (a *app) View() tea.View { return tea.NewView(a.modal.View()) }

func TestProgramUsesACommitAsBase(t *testing.T) {
	f := newFake()
	m := New(t.Context(), f, repo, "main", baseNone, testKeys(), WithConfig(testConfig()), withClock(testNow))
	m.SetTheme(testTheme())
	ready := make(chan struct{})
	a := &app{modal: m, ready: ready}
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(wideW, wideH))
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("the graph didn't load")
	}
	tm.Send(press("j"))
	tm.Send(press("b"))
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*app)
	if !ok || final.base == nil {
		t.Fatal("the program ended without a base")
	}
	want := ui.BaseMsg{Repo: repo, Ref: main1, Label: "main @ " + short(main1), Branch: "main"}
	if *final.base != want || final.shown != ui.FilesTitle {
		t.Errorf("base %+v, shown %q; want %+v and the files", *final.base, final.shown, want)
	}
}
