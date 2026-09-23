package pulls

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// app hosts the section as the program root, the way the tui does. It
// reports every finished change on done.
type app struct {
	s    *Section
	done chan ui.DoneMsg
}

func (a app) Init() tea.Cmd {
	a.s.Update(ui.RepoMsg{Repo: repo})
	return a.s.Init()
}

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.s.SetSize(msg.Width, msg.Height)
		return a, nil
	case tea.KeyPressMsg:
		if msg.String() == "q" {
			return a, tea.Quit
		}
	case ui.DoneMsg:
		cmd := a.s.Update(msg)
		a.done <- msg
		return a, cmd
	}
	return a, a.s.Update(msg)
}

func (a app) View() tea.View { return tea.NewView(a.s.View()) }

func TestProgramOpensGoesBackAndMerges(t *testing.T) {
	svc := newFakeService()
	s := newTest(t, svc, 80, 24)
	done := make(chan ui.DoneMsg, 1)
	tm := teatest.NewTestModel(t, app{s: s, done: done}, teatest.WithInitialTermSize(80, 24))
	wait := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(3*time.Second))
	}

	wait("Retry GraphQL requests")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	wait("Does this survive a crash")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEscape})
	wait("Add a disk layer")
	tm.Type("m")
	select {
	case msg := <-done:
		if msg.What != "merge #135" || msg.Err != nil {
			t.Errorf("done = %+v, want merge #135 without error", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the merge never finished")
	}
	// Once the merge is confirmed, the open list no longer has it.
	wait("Bump charm.land")
	tm.Type("q")

	final := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(app).s
	if got := svc.state(135).State; got != core.StateMerged {
		t.Errorf("#135 is %s, want merged", got)
	}
	if final.thread != nil {
		t.Error("the detail is still open")
	}
	if pr, _ := final.feed.Selected(); pr.Number == 135 {
		t.Error("the merged pull request is still in the open list")
	}
}
