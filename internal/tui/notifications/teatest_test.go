package notifications

import (
	"bytes"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// app hosts the section as the program root, the way the tui would: it
// hands DoneMsg back to the section, gives the keys to a question while
// one is open, and records what it would open.
type app struct {
	s      *Section
	ask    *ui.ConfirmModal
	opened []string
	width  int
	// done hears of every change GitHub answered. The test waits on it
	// rather than on output: the question closes in a command of its own,
	// so a frame may come between it and the answer, and the terminal then
	// redraws only the cells that changed.
	done chan struct{}
}

func (a *app) Init() tea.Cmd { return a.s.Init() }

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.s.SetSize(msg.Width, msg.Height-1)
		return a, nil
	case tea.KeyPressMsg:
		if msg.String() == "q" {
			return a, tea.Quit
		}
		if a.ask != nil {
			return a, a.ask.Update(msg)
		}
	case ui.OpenModalMsg:
		if a.ask, _ = msg.Modal.(*ui.ConfirmModal); a.ask != nil {
			a.ask.SetSize(a.width, 1)
		}
		return a, nil
	case ui.DoneMsg:
		a.done <- struct{}{}
	case ui.CloseModalMsg:
		a.ask = nil
		return a, nil
	case ui.OpenMsg, ui.OpenPullMsg:
		o, _ := opened(msg)
		a.opened = append(a.opened, o)
		return a, nil
	case filterform.AppliedMsg:
		// The app applies what its filter modal sends.
		return a, a.s.ApplyFilter(msg)
	}
	return a, a.s.Update(msg)
}

// View shows the section over a line that holds the question, if one is
// open.
func (a *app) View() tea.View {
	last := ""
	if a.ask != nil {
		last = a.ask.View()
	}
	return tea.NewView(a.s.View() + "\n" + last)
}

// wait bounds each step of a program test. It is generous since a loaded
// machine runs the program slowly, and a passing test never waits it out.
const wait = 30 * time.Second

func TestProgram(t *testing.T) {
	svc := newFake(inbox()...)
	s := New(t.Context(), svc, config.Keymap{
		config.ContextGlobal: {"select": {"enter"}, "refresh": {"r"}},
		"notifications":      {"filter": {"f"}, "done": {"d"}, "bottom": {"end"}},
		"confirm":            {"yes": {"y"}, "no": {"n"}},
	}, WithNow(func() time.Time { return now }))
	s.Focus()
	a := &app{s: s, done: make(chan struct{}, 2)}
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(80, 10))

	waitFor := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(wait))
	}
	waitFor("Add a renderer")

	// Open the first thread, which marks it read and drops it from the
	// unread inbox, then show every thread and mark the last one done.
	tm.Send(keyPress("enter"))
	tm.Send(filterform.AppliedMsg{Query: ""})
	waitFor("Moderate severity")
	tm.Send(keyPress("end"))
	tm.Send(keyPress("d"))
	waitFor(`Mark "Moderate severity`)
	tm.Send(keyPress("y"))
	for range 2 {
		select {
		case <-a.done:
		case <-time.After(wait):
			t.Fatal("GitHub didn't answer the read and the mark done")
		}
	}
	tm.Send(keyPress("q"))

	final := tm.FinalModel(t, teatest.WithFinalTimeout(wait)).(*app)
	if want := []string{"pull charmbracelet/bubbletea#1"}; !slices.Equal(final.opened, want) {
		t.Errorf("opened %q, want %q", final.opened, want)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if !slices.Equal(svc.reads, []string{"1"}) || !slices.Equal(svc.dones, []string{"7"}) {
		t.Errorf("reads %q, dones %q; want [1] and [7]", svc.reads, svc.dones)
	}
	if !final.s.All() {
		t.Error("the filter should show every thread")
	}
}
