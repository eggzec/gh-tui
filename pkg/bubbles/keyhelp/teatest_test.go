package keyhelp

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// host shows the help the way the root would: it focuses it, and quits
// when the help closes.
type host struct {
	help   Model
	closed bool
}

func (h host) Init() tea.Cmd { return nil }

func (h host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.help.SetSize(msg.Width, msg.Height)
		return h, nil
	case CloseMsg:
		if msg.ID == h.help.ID() {
			h.closed = true
			return h, tea.Quit
		}
	}
	var cmd tea.Cmd
	h.help, cmd = h.help.Update(msg)
	return h, cmd
}

func (h host) View() tea.View { return tea.NewView(h.help.View()) }

// tab, then ctrl+r, lists the bindings of ctrl+r; tab stops capturing and
// keeps them; esc clears the key, and esc again closes the help.
func TestProgramCapture(t *testing.T) {
	m := New(WithLayers(layers()))
	m.Focus()
	tm := teatest.NewTestModel(t, host{help: m}, teatest.WithInitialTermSize(80, 20))
	wait := func(s string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(s))
		}, teatest.WithDuration(5*time.Second))
	}
	wait("12 keys")
	tm.Send(tab)
	wait("Press a key to find it")
	tm.Send(ctrlR)
	wait("2 of 12")
	tm.Send(tab)
	wait("key ctrl+r")
	tm.Send(esc)
	wait("12 keys")
	tm.Send(esc)
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(host)
	if !ok {
		t.Fatal("final model is not a host")
	}
	if !final.closed || final.help.Key() != "" {
		t.Errorf("closed %v, key %q; want closed with the key cleared", final.closed, final.help.Key())
	}
}
