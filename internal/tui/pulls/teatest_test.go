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

// app hosts the section as the program root, the way the tui does, and
// draws the open modal in place of the section. It reports every finished
// change on done.
type app struct {
	h    *host
	done chan ui.DoneMsg
}

func (a app) Init() tea.Cmd {
	a.h.Update(ui.RepoMsg{Repo: repo})
	return a.h.Init()
}

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.h.SetSize(msg.Width, msg.Height)
		for _, m := range a.h.modals {
			m.SetSize(msg.Width, msg.Height)
		}
		return a, nil
	case tea.KeyPressMsg:
		if msg.String() == "q" && len(a.h.modals) == 0 {
			return a, tea.Quit
		}
	case ui.DoneMsg:
		cmd := a.h.Update(msg)
		a.done <- msg
		return a, cmd
	}
	return a, a.h.Update(msg)
}

func (a app) View() tea.View {
	if m := a.h.modal(); m != nil {
		return tea.NewView(m.View())
	}
	return tea.NewView(a.h.View())
}

func TestProgramOpensGoesBackAndMerges(t *testing.T) {
	svc := newFakeService()
	h := newTest(t, svc, 80, 24)
	done := make(chan ui.DoneMsg, 1)
	tm := teatest.NewTestModel(t, app{h: h, done: done}, teatest.WithInitialTermSize(80, 24))
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

	final := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(app).h
	if got := svc.state(135).State; got != core.StateMerged {
		t.Errorf("#135 is %s, want merged", got)
	}
	if final.modal() != nil {
		t.Error("the modal is still open")
	}
	if pr, _ := final.feed.Selected(); pr.Number == 135 {
		t.Error("the merged pull request is still in the open list")
	}
}
