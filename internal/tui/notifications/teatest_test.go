package notifications

import (
	"bytes"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// app hosts the section as the program root, the way the tui would: it
// hands DoneMsg back to the section and records what it would open.
type app struct {
	s      *Section
	opened []string
}

func (a *app) Init() tea.Cmd { return a.s.Init() }

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.s.SetSize(msg.Width, msg.Height)
		return a, nil
	case tea.KeyPressMsg:
		if msg.String() == "q" {
			return a, tea.Quit
		}
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

func (a *app) View() tea.View { return tea.NewView(a.s.View()) }

func TestProgram(t *testing.T) {
	svc := newFake(inbox()...)
	s := New(t.Context(), svc, map[string][]string{
		"select": {"enter"}, "filter": {"f"}, "mark_done": {"d"}, "refresh": {"r"},
	}, WithNow(func() time.Time { return now }))
	s.Focus()
	tm := teatest.NewTestModel(t, &app{s: s}, teatest.WithInitialTermSize(80, 10))

	waitFor := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(5*time.Second))
	}
	waitFor("Add a renderer")

	// Open the first thread, which marks it read and drops it from the
	// unread inbox, then show every thread and mark the last one done.
	tm.Send(keyPress("enter"))
	tm.Send(filterform.AppliedMsg{Query: ""})
	waitFor("Moderate severity")
	tm.Send(keyPress("end"))
	tm.Send(keyPress("d"))
	tm.Send(keyPress("q"))

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*app)
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
