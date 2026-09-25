package logview

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// host shows a log view the way a parent modal would: it sizes the view,
// and quits when the view asks to close.
type host struct {
	log    Model
	closed bool
}

func (h host) Init() tea.Cmd { return nil }

func (h host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.log.SetSize(msg.Width, msg.Height)
		return h, nil
	case CloseMsg:
		if msg.ID == h.log.ID() {
			h.closed = true
			return h, tea.Quit
		}
	}
	var cmd tea.Cmd
	h.log, cmd = h.log.Update(msg)
	return h, cmd
}

func (h host) View() tea.View { return tea.NewView(h.log.View()) }

// The program's messages arrive in order, so the final model tells what
// the keys did; the output bytes depend on how the renderer batched them.
func TestProgram(t *testing.T) {
	m := open(t, WithFocusFailed(true))
	tm := teatest.NewTestModel(t, host{log: m}, teatest.WithInitialTermSize(80, 16))

	// Collapse everything, then find the second error again.
	tm.Send(press("="))
	tm.Send(press("e"))
	tm.Send(press("e"))
	tm.Send(press("t"))
	tm.Send(press("/"))
	tm.Type("Get")
	tm.Send(press("enter"))
	// The first esc clears the search, the second closes.
	tm.Send(press("esc"))
	tm.Send(press("esc"))

	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(host)
	if !ok {
		t.Fatal("final model is not a host")
	}
	got := final.log
	if !final.closed || got.Query() != "" {
		t.Errorf("closed %v with query %q; want closed with the search cleared", final.closed, got.Query())
	}
	if got.jumped != Error || got.at != 1 {
		t.Errorf("last jump went to %v %d, want error 2", got.jumped, got.at+1)
	}
	if want := "func commandClipboardBackend.Get is unused"; !strings.Contains(cursorText(got), want) {
		t.Errorf("cursor on %q, want the match of Get", cursorText(got))
	}
	if got.TimeMode() != TimeRelative || got.Width() != 80 || got.Height() != 16 {
		t.Errorf("times %v at %dx%d, want relative at 80x16", got.TimeMode(), got.Width(), got.Height())
	}
}
