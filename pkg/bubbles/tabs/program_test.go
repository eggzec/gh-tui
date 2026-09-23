package tabs

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// parent is a minimal program root that owns a tab bar and loads sections
// on ChangeMsg, the way the tui does.
type parent struct {
	tabs   Model
	loaded []int
}

func (p parent) Init() tea.Cmd { return nil }

func (p parent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.tabs.SetWidth(msg.Width)
		return p, nil
	case tea.KeyPressMsg:
		if msg.String() == "q" {
			return p, tea.Quit
		}
	case ChangeMsg:
		if msg.ID == p.tabs.ID() {
			p.loaded = append(p.loaded, msg.Index)
		}
		return p, nil
	}
	var cmd tea.Cmd
	p.tabs, cmd = p.tabs.Update(msg)
	return p, cmd
}

func (p parent) View() tea.View {
	return tea.NewView(fmt.Sprintf("%s\nloaded %d", p.tabs.View(), len(p.loaded)))
}

func TestProgramSwitchesTabs(t *testing.T) {
	other := New(WithTabs(sections...))
	p := parent{tabs: New(WithTabs(sections...), WithFocused(true))}
	tm := teatest.NewTestModel(t, p, teatest.WithInitialTermSize(80, 24))

	tm.Send(keyTab)
	tm.Send(keyTab)
	tm.Send(digit('2')) // back to Issues
	tm.Send(digit('2')) // same tab, no load
	tm.Send(keyShiftTab)
	tm.Send(keyShiftTab) // wraps to Repositories
	tm.Send(ChangeMsg{ID: other.ID(), Index: 1})

	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("loaded 5"))
	}, teatest.WithDuration(2*time.Second))
	tm.Send(digit('q'))

	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second)).(parent)
	if !ok {
		t.Fatal("final model is not a parent")
	}
	if got := final.tabs.Active(); got != 3 {
		t.Errorf("Active() = %d, want 3", got)
	}
	if len(final.loaded) != 5 {
		t.Errorf("loaded %v, want five sections", final.loaded)
	}
}
