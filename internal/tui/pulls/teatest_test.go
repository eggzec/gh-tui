package pulls

import (
	"bytes"
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// app hosts the section as the program root, the way the tui does, and
// draws the open modal in place of the section. It reports every finished
// change on done.
type app struct {
	h    *host
	done chan ui.DoneMsg
	// applied, if set, gets the filters applied in the filter modal, which
	// f and s open as the tui does, and modals a value once a modal opens
	// or closes.
	applied chan filterform.AppliedMsg
	modals  chan struct{}
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
		if len(a.h.modals) > 0 {
			break
		}
		switch msg.String() {
		case "q":
			return a, tea.Quit
		case "f", "s":
			tab := filterform.FiltersTab
			if msg.String() == "s" {
				tab = filterform.SortTab
			}
			if f, ok := a.h.Filter(); ok {
				return a, ui.OpenModal(ui.NewFilterModal(context.Background(), a.h.Title(), a.h.Section, f, ui.OnTab(tab)))
			}
		}
	case ui.OpenModalMsg, ui.CloseModalMsg:
		cmd := a.h.Update(msg)
		if a.modals != nil {
			a.modals <- struct{}{}
		}
		return a, cmd
	case filterform.AppliedMsg:
		cmd := a.h.Update(msg)
		if a.applied != nil {
			a.applied <- msg
		}
		return a, cmd
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
	if n := len(a.h.modals); n > 0 {
		return tea.NewView(a.h.modals[n-1].View())
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

func TestProgramSwitchesTabsAndFilters(t *testing.T) {
	svc := newFakeService()
	h := newTest(t, svc, 80, 24)
	applied, modals := make(chan filterform.AppliedMsg, 1), make(chan struct{}, 1)
	tm := teatest.NewTestModel(t, app{h: h, done: make(chan ui.DoneMsg, 1), applied: applied, modals: modals}, teatest.WithInitialTermSize(80, 24))
	waitModal := func(what string) {
		t.Helper()
		select {
		case <-modals:
		case <-time.After(3 * time.Second):
			t.Fatalf("the filter modal never %s", what)
		}
	}
	wait := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(3*time.Second))
	}
	wait("Retry GraphQL requests")

	// ] shows the merged tab two tabs on, and the query line, above the
	// first row, takes an author.
	tm.Type("]]f")
	waitModal("opened")
	// Reading the output up to the form drops the rows of the merged tab
	// drawn before it, which the filtered list has too.
	wait("Assignee")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyUp})
	tm.Type(" author:hubot")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	select {
	case msg := <-applied:
		if msg.Query != "is:merged author:hubot sort:updated-desc" {
			t.Errorf("applied %q, want the tab and the author", msg.Query)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the filter was never applied")
	}
	// q is typed into the form until it closes, and the filtered list loads
	// in a command, which q may beat.
	waitModal("closed")
	wait("Rename the watch package")
	tm.Type("q")

	final := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(app).h
	if final.tab != core.StateMerged || final.query != "author:hubot" || len(final.modals) != 0 {
		t.Errorf("tab %q, query %q, %d modals; want merged by hubot, the modal closed", final.tab, final.query, len(final.modals))
	}
	if pr, ok := final.feed.Selected(); !ok || pr.Number != 79 {
		t.Errorf("selected #%d, want #79, the only one merged by hubot", pr.Number)
	}
}

func TestProgramSorts(t *testing.T) {
	svc := newFakeService()
	h := newTest(t, svc, 80, 24)
	applied, modals := make(chan filterform.AppliedMsg, 1), make(chan struct{}, 1)
	tm := teatest.NewTestModel(t, app{h: h, done: make(chan ui.DoneMsg, 1), applied: applied, modals: modals}, teatest.WithInitialTermSize(80, 24))
	waitModal := func(what string) {
		t.Helper()
		select {
		case <-modals:
		case <-time.After(3 * time.Second):
			t.Fatalf("the filter modal never %s", what)
		}
	}
	wait := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(3*time.Second))
	}
	wait("Retry GraphQL requests")

	// s opens the Sort tab, where right sorts by the next option, and
	// down and right flip its order.
	tm.Type("s")
	waitModal("opened")
	wait("Oldest first")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyRight})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyRight})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	select {
	case msg := <-applied:
		if msg.Query != "is:open sort:created-asc" {
			t.Errorf("applied %q, want the open ones, oldest created first", msg.Query)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the sort was never applied")
	}
	waitModal("closed")
	teatest.WaitFor(t, tm.Output(), func([]byte) bool {
		q := svc.listed()
		return len(q) > 0 && q[len(q)-1].Filter == "sort:created-asc"
	}, teatest.WithDuration(3*time.Second))
	tm.Type("q")

	final := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second)).(app).h
	if final.tab != core.StateOpen || final.query != "sort:created-asc" || len(final.modals) != 0 {
		t.Errorf("tab %q, query %q, %d modals; want the open ones sorted, the modal closed", final.tab, final.query, len(final.modals))
	}
	if q := svc.listed(); q[len(q)-1] != (pulls.ListQuery{Repo: repo, State: core.StateOpen, Filter: "sort:created-asc"}) {
		t.Errorf("the service listed %+v last, want the open ones sorted", q[len(q)-1])
	}
}
