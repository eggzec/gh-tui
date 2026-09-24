package calendar

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// app hosts a calendar as the program root, the way the tui would, and
// quits on q. It doesn't check SelectMsg: that comes from a command, which
// may still be running when q quits; the Update tests cover it.
type app struct {
	cal Model
}

func (a app) Init() tea.Cmd {
	return a.cal.Init()
}

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.cal.SetSize(msg.Width, msg.Height)
		return a, nil
	case tea.KeyPressMsg:
		if msg.String() == "q" {
			return a, tea.Quit
		}
	}
	var cmd tea.Cmd
	a.cal, cmd = a.cal.Update(msg)
	return a, cmd
}

func (a app) View() tea.View {
	return tea.NewView(a.cal.View())
}

func TestProgram(t *testing.T) {
	m := New(WithWeeks(year(today)), WithFocused(true))
	tm := teatest.NewTestModel(t, app{cal: m}, teatest.WithInitialTermSize(80, 10))
	for _, k := range []string{"h", "h", "k", "q"} {
		tm.Send(press(k))
	}
	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(app)
	want := date(2026, 9, 9)
	if got := selected(t, final.cal); !got.Equal(want) {
		t.Fatalf("cursor on %v, want %v", got, want)
	}
	if final.cal.Width() != 80 {
		t.Fatalf("width %d, want the terminal's", final.cal.Width())
	}
}
