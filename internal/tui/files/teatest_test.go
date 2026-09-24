package files

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

// app hosts the section as the program root, standing in for the tui: it
// records the app messages and ends the program once one opens a page in
// the browser.
type app struct {
	section *Section
	got     []tea.Msg
}

func (a *app) Init() tea.Cmd {
	return a.section.Init()
}

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.section.SetSize(msg.Width, msg.Height)
		return a, nil
	case ui.OpenMsg:
		a.got = append(a.got, msg)
		return a, tea.Quit
	}
	return a, a.section.Update(msg)
}

func (a *app) View() tea.View {
	return tea.NewView(a.section.View())
}

func TestProgram(t *testing.T) {
	s := New(t.Context(), sampleFake(), config.Default().Keys)
	s.SetTheme(testTheme())
	s.Focus()
	a := &app{section: s}
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(40, 12))

	waitFor := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(5*time.Second))
	}
	waitFor("No repository selected")
	tm.Send(ui.RepoMsg{Repo: ghTUI})
	waitFor("AGENTS.md")

	// Expand cmd and cmd/gh-tui, then open main.go in the browser.
	tm.Send(press("+"))
	waitFor("gh-tui")
	tm.Send(press("down"))
	tm.Send(press("l"))
	waitFor("main.go")
	tm.Send(press("down"))
	tm.Send(press("o"))

	final, _ := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*app)
	want := []tea.Msg{ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui/blob/HEAD/cmd/gh-tui/main.go"}}
	if !slices.Equal(final.got, want) {
		t.Errorf("messages = %#v, want %#v", final.got, want)
	}
}
