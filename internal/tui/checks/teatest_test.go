package checks

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// app hosts the step as the program root, standing in for the modal of the
// pull request: it ends once the step asks to close.
type app struct {
	step *Step
	// loaded is closed once the checks show, and job once the log of the
	// failed job does.
	loaded, job chan struct{}
}

func (a *app) Init() tea.Cmd { return a.step.Init() }

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.step.SetSize(msg.Width, msg.Height)
		return a, nil
	case CloseMsg:
		return a, tea.Quit
	case ui.OpenMsg:
		return a, nil
	}
	cmd := a.step.Update(msg)
	if a.loaded != nil && a.step.loaded {
		close(a.loaded)
		a.loaded = nil
	}
	if a.job != nil && a.step.view.State() == jobview.Ready {
		close(a.job)
		a.job = nil
	}
	return a, cmd
}

func (a *app) View() tea.View { return tea.NewView(a.step.View()) }

func wait(t *testing.T, ch chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s didn't happen", what)
	}
}

func TestProgramDrillsInAndRerunsTheFailedJobs(t *testing.T) {
	f := newFake()
	s := New(t.Context(), f, repo, query.Number, config.Default().Keys,
		forTests(), WithClock(func() time.Time { return testNow }), WithIcons(ui.NewIcons(config.IconsUnicode)))
	s.SetTheme(testTheme())
	loaded, job := make(chan struct{}), make(chan struct{})
	a := &app{step: s, loaded: loaded, job: job}
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(wideW, wideH))
	wait(t, loaded, "the checks loading")
	tm.Send(press("enter"))
	wait(t, job, "the log of the failed job loading")
	tm.Send(press("ctrl+r"))
	tm.Send(press("y"))
	tm.Send(press("esc"))
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
	if final.step.mode != listMode {
		t.Errorf("the program ended in mode %d, want the checks", final.step.mode)
	}
}
