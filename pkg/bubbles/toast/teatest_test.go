package toast

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// host is a minimal parent that pushes a toast on "p", forwards everything
// else to the stack, and shows the stack.
type host struct {
	toasts        Model
	width, height int
}

func (h host) Init() tea.Cmd { return nil }

func (h host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.width, h.height = msg.Width, msg.Height
		h.toasts.SetSize(msg.Width, msg.Height)
		return h, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "p":
			return h, h.toasts.Push(Error, "Could not merge #42")
		case "q":
			return h, tea.Quit
		}
	}
	var cmd tea.Cmd
	h.toasts, cmd = h.toasts.Update(msg)
	return h, cmd
}

func (h host) View() tea.View {
	return tea.NewView(h.toasts.View())
}

func TestPushAndDismissInAProgram(t *testing.T) {
	// A long duration, so that only the key removes the toast.
	tm := teatest.NewTestModel(t, host{toasts: New(WithErrorDuration(time.Hour))},
		teatest.WithInitialTermSize(80, 24))
	tm.Type("pp")
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Could not merge #42")) && bytes.Contains(out, []byte("×2"))
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	tm.Type("q")
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second)).(host)
	if !ok {
		t.Fatal("final model is not a host")
	}
	if !final.toasts.Empty() {
		t.Errorf("toasts left after dismiss: %v", entries(final.toasts))
	}
}
