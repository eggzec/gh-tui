package repos

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
// records the app messages and passes everything back. Showing another
// section ends the program, since this one would be hidden.
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
	case ui.ShowMsg:
		a.got = append(a.got, msg)
		return a, tea.Quit
	case ui.RepoMsg, ui.DoneMsg:
		a.got = append(a.got, msg)
	}
	return a, a.section.Update(msg)
}

func (a *app) View() tea.View {
	return tea.NewView(a.section.View())
}

func TestProgram(t *testing.T) {
	repos := sampleRepos()
	for i := range repos {
		repos[i].Starred = false
	}
	f := newFake(repos...)
	s := New(t.Context(), f, config.Default().Keys, WithNow(func() time.Time { return testNow }))
	s.SetTheme(testTheme())
	s.Focus()
	a := &app{section: s}
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(80, 10))

	waitFor := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(5*time.Second))
	}
	waitFor("eggzec/dotfiles")

	// Star the third repo; its row redraws with the only filled star.
	tm.Send(press("down"))
	tm.Send(press("down"))
	tm.Send(press("s"))
	waitFor("★")
	tm.Send(press("enter"))

	final, _ := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*app)
	if !f.starred(ref("eggzec/dotfiles")) {
		t.Error("eggzec/dotfiles isn't starred")
	}
	// The star is sent in the background, so it may finish before or
	// after the choice.
	done := tea.Msg(ui.DoneMsg{From: ui.ReposTitle, What: "star eggzec/dotfiles"})
	if !slices.Contains(final.got, done) {
		t.Errorf("messages = %#v, want %#v among them", final.got, done)
	}
	chosen := slices.DeleteFunc(slices.Clone(final.got), func(m tea.Msg) bool { return m == done })
	want := []tea.Msg{
		ui.RepoMsg{Repo: ref("eggzec/dotfiles")},
		ui.ShowMsg{Title: "Pull requests"},
	}
	if !slices.Equal(chosen, want) {
		t.Errorf("messages = %#v, want %#v", chosen, want)
	}
}
