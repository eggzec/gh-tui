package actions

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// app hosts the modal as the program root, standing in for the tui: it
// ends once the modal closes.
type app struct {
	modal *Modal
	// ready is closed once the log of the failed job shows.
	ready chan struct{}
	done  []ui.DoneMsg
}

func (a *app) Init() tea.Cmd { return a.modal.Init() }

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.modal.SetSize(msg.Width, msg.Height)
		return a, nil
	case ui.CloseModalMsg:
		return a, tea.Quit
	case ui.DoneMsg:
		a.done = append(a.done, msg)
	}
	cmd := a.modal.Update(msg)
	if a.ready != nil && a.modal.log.State() == jobview.Ready {
		close(a.ready)
		a.ready = nil
	}
	return a, cmd
}

func (a *app) View() tea.View { return tea.NewView(a.modal.View()) }

func TestProgramRerunsTheFailedJobs(t *testing.T) {
	f := newFake()
	m := New(t.Context(), f, repo, testKeys(), forTests(), withIcons())
	m.SetTheme(testTheme())
	ready := make(chan struct{})
	a := &app{modal: m, ready: ready}
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(wideW, wideH))
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("the log didn't load")
	}
	tm.Send(press("R"))
	tm.Send(press("y"))
	tm.Send(press("esc"))
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*app)
	if !ok {
		t.Fatal("the program ended without its model")
	}
	f.mu.Lock()
	sent := slices.Clone(f.sent)
	f.mu.Unlock()
	if !slices.Equal(sent, []string{"rerun failed"}) {
		t.Errorf("sent %v, want the failed jobs re-run", sent)
	}
	if final.modal.run.ID != failedRun {
		t.Errorf("the program ended on run %d", final.modal.run.ID)
	}
}
