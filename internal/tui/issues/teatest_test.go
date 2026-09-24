package issues

import (
	"bytes"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// app hosts the section as the program root, the way the tui would, and
// draws the open modal in place of the section.
type app struct {
	h *host
}

func (a app) Init() tea.Cmd {
	return tea.Batch(a.h.Update(ui.RepoMsg{Repo: testRepo}), a.h.Init())
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
		// The app quits on q unless a modal is taking every key.
		if msg.String() == "q" && len(a.h.modals) == 0 {
			return a, tea.Quit
		}
	}
	return a, a.h.Update(msg)
}

func (a app) View() tea.View {
	if m := a.h.modal(); m != nil {
		return tea.NewView(m.View())
	}
	return tea.NewView(a.h.View())
}

func TestProgram(t *testing.T) {
	svc := newFakeService(sampleIssues(40))
	svc.addComments(999, sampleComments(3)...)
	svc.gate = make(chan struct{})
	s := New(t.Context(), svc, config.Default().Keys, WithNow(func() time.Time { return testNow }))
	s.SetTheme(testTheme())
	s.Focus()
	tm := teatest.NewTestModel(t, app{h: &host{Section: s}}, teatest.WithInitialTermSize(80, 16))

	waitFor := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(5*time.Second))
	}
	waitFor("Crash when the config file")

	tm.Send(keyMsg("down"))
	tm.Send(keyMsg("enter"))
	waitFor("I can reproduce this")

	tm.Send(keyMsg("esc"))
	waitFor("Notifications tab keeps pol")

	// Closing #999 in the open list shows it closed at once. Once GitHub
	// agrees, the list no longer has it.
	tm.Send(keyMsg("x"))
	waitFor("✓")
	close(svc.gate)
	waitFor("8 ● Notifications tab keeps pol")
	tm.Send(keyMsg("q"))

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(app)
	if got := svc.changeCalls(); !slices.Equal(got, []string{"close 999"}) {
		t.Errorf("changes = %v, want [close 999]", got)
	}
	if it, _ := final.h.list.Selected(); it.Number != 998 {
		t.Errorf("selected #%d after the close, want #998", it.Number)
	}
}

func TestProgramComment(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, sampleComments(3)...)
	s := New(t.Context(), svc, config.Default().Keys, WithNow(func() time.Time { return testNow }))
	s.SetTheme(testTheme())
	s.Focus()
	tm := teatest.NewTestModel(t, app{h: &host{Section: s}}, teatest.WithInitialTermSize(80, 40))
	waitFor := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(5*time.Second))
	}
	waitFor("Crash when the config file")
	tm.Send(keyMsg("down"))
	tm.Send(keyMsg("enter"))
	waitFor("I can reproduce this")

	// The q and x in the comment are text, not quit and close.
	tm.Send(keyMsg("c"))
	waitFor("Comment on #999")
	tm.Type("quite fixed")
	tm.Send(keyMsg("ctrl+s"))
	// GitHub's comment replaces the pending one at the end of the thread,
	// which the terminal is tall enough to show whole.
	waitFor("octocat")
	tm.Send(keyMsg("esc"))
	waitFor("Notifications tab keeps pol")
	tm.Send(keyMsg("q"))

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(app)
	if got := svc.changeCalls(); !slices.Equal(got, []string{"comment 999: quite fixed"}) {
		t.Errorf("changes = %v, want the comment only", got)
	}
	if final.h.modal() != nil {
		t.Error("the modal should be closed")
	}
}
