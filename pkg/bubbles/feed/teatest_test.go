package feed

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// app hosts a feed as the program root, the way the tui would.
type app struct {
	feed Model[item]
}

func (a app) Init() tea.Cmd {
	return a.feed.Init()
}

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.feed.SetSize(msg.Width, msg.Height)
		return a, nil
	case tea.KeyPressMsg:
		if msg.String() == "q" {
			return a, tea.Quit
		}
	}
	var cmd tea.Cmd
	a.feed, cmd = a.feed.Update(msg)
	return a, cmd
}

func (a app) View() tea.View {
	return tea.NewView(a.feed.View())
}

func TestProgram(t *testing.T) {
	src := newSource(100, 20)
	m := newModel(src.fetch, renderItem, WithFocused(true))
	tm := teatest.NewTestModel(t, app{feed: m}, teatest.WithInitialTermSize(80, 10))

	waitFor := func(s string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(s))
		}, teatest.WithDuration(5*time.Second))
	}
	waitFor("#9 item 9")

	// The renderer only redraws changed cells, so wait for new rows rather
	// than for the cursor.
	for range 5 {
		tm.Send(press("down"))
	}
	// The end of the first chunk loads the second one below it.
	tm.Send(press("end"))
	waitFor("#20 item 20")
	tm.Send(press("down"))
	tm.Send(press("q"))

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(app)
	if got := final.feed.Index(); got != 20 {
		t.Fatalf("Index() = %d, want 20", got)
	}
	if it, ok := final.feed.Selected(); !ok || it.id != "20" {
		t.Fatalf("Selected() = %v, %v; want item 20", it, ok)
	}
}
