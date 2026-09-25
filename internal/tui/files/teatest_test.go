package files

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// app hosts the section as the program root, standing in for the tui: it
// shows the modal the section opens in place of the section, and ends the
// program once a page opens in the browser.
type app struct {
	section *Section
	modal   ui.Modal
	width   int
	height  int
	got     []tea.Msg
}

func (a *app) Init() tea.Cmd {
	return a.section.Init()
}

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.section.SetSize(msg.Width, msg.Height)
		return a, nil
	case ui.OpenMsg:
		a.got = append(a.got, msg)
		return a, tea.Quit
	case ui.OpenModalMsg:
		a.modal = msg.Modal
		a.modal.SetTheme(testTheme())
		a.modal.SetSize(a.width, a.height)
		return a, nil
	case ui.CloseModalMsg:
		if msg.Modal == a.modal {
			a.modal = nil
		}
		return a, nil
	case tea.KeyPressMsg:
		if a.modal != nil {
			return a, a.modal.Update(msg)
		}
		if msg.String() == "ctrl+p" {
			// The find-file key of the app.
			mod, cmd := a.section.FindFile()
			if mod != nil {
				a.modal = mod
				a.modal.SetTheme(testTheme())
				a.modal.SetSize(a.width, a.height)
			}
			return a, cmd
		}
		return a, a.section.Update(msg)
	}
	cmd := a.section.Update(msg)
	if a.modal != nil {
		cmd = tea.Batch(cmd, a.modal.Update(msg))
	}
	return a, cmd
}

func (a *app) View() tea.View {
	if a.modal != nil {
		return tea.NewView(a.modal.View())
	}
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

	// Expand cmd and cmd/gh-tui, preview main.go, then open it in the
	// browser.
	tm.Send(press("+"))
	waitFor("gh-tui")
	tm.Send(press("down"))
	tm.Send(press("l"))
	waitFor("main.go")
	tm.Send(press("down"))
	tm.Send(press("enter"))
	waitFor("hello")
	tm.Send(press("o"))

	final, _ := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*app)
	if final.modal == nil || final.modal.Title() != "cmd/gh-tui/main.go" {
		t.Errorf("modal = %v, want the preview of main.go", final.modal)
	}
	want := []tea.Msg{ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui/blob/HEAD/cmd/gh-tui/main.go"}}
	if !slices.Equal(final.got, want) {
		t.Errorf("messages = %#v, want %#v", final.got, want)
	}
}

// TestProgramFinder finds a file by typing some of its path, opens it in
// the preview, goes back to the finder with esc, and shows the file in the
// tree.
func TestProgramFinder(t *testing.T) {
	s := New(t.Context(), sampleFake(), config.Default().Keys, WithRepo(ghTUI))
	s.SetTheme(testTheme())
	s.Focus()
	a := &app{section: s}
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(60, 12))
	waitFor := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(5*time.Second))
	}
	waitFor("AGENTS.md")
	tm.Send(press("ctrl+p"))
	waitFor("6 files")
	tm.Type("ghmain")
	waitFor("1 match")
	tm.Send(press("enter"))
	waitFor("hello")
	tm.Send(press("esc"))
	waitFor("ghmain")
	tm.Send(press("ctrl+t"))
	// The tree shows the file once cmd and cmd/gh-tui are expanded, and
	// only then may o open it.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Count(ansi.Strip(string(b)), "▾") >= 2
	}, teatest.WithDuration(5*time.Second))
	tm.Send(press("o"))

	final, _ := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*app)
	if final.modal != nil {
		t.Errorf("modal %q open, want the tree", final.modal.Title())
	}
	if got := final.section.selected().Path; got != "cmd/gh-tui/main.go" {
		t.Errorf("tree cursor on %q", got)
	}
	want := []tea.Msg{ui.OpenMsg{URL: "https://github.com/eggzec/gh-tui/blob/HEAD/cmd/gh-tui/main.go"}}
	if !slices.Equal(final.got, want) {
		t.Errorf("messages = %#v, want %#v", final.got, want)
	}
}
