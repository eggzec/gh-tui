package calendar

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// app hosts a calendar as the program root, the way the tui would. It keeps
// the day of the last SelectMsg and quits on q.
type app struct {
	cal      Model
	selected time.Time
}

func (a app) Init() tea.Cmd {
	return a.cal.Init()
}

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.cal.SetSize(msg.Width, msg.Height)
		return a, nil
	case SelectMsg:
		if msg.ID == a.cal.ID() {
			a.selected = msg.Day.Date
		}
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
	if !final.selected.Equal(want) {
		t.Fatalf("last SelectMsg for %v, want %v", final.selected, want)
	}
	if final.cal.Width() != 80 {
		t.Fatalf("width %d, want the terminal's", final.cal.Width())
	}
}
