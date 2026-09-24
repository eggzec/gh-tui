package pager

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// host shows a pager the way a parent modal would: it sets the content,
// focuses the pager, and quits when the pager asks to close.
type host struct {
	pager  Model
	closed bool
	// highlight is the command SetContent returned.
	highlight tea.Cmd
}

func (h host) Init() tea.Cmd { return h.highlight }

func (h host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.pager.SetSize(msg.Width, msg.Height)
		return h, nil
	case CloseMsg:
		if msg.ID == h.pager.ID() {
			h.closed = true
			return h, tea.Quit
		}
	}
	var cmd tea.Cmd
	h.pager, cmd = h.pager.Update(msg)
	return h, cmd
}

func (h host) View() tea.View { return tea.NewView(h.pager.View()) }

func TestProgram(t *testing.T) {
	p := New()
	highlight := p.SetContent("main.go", goSource)
	p.Focus()
	tm := teatest.NewTestModel(t, host{pager: p, highlight: highlight}, teatest.WithInitialTermSize(60, 12))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("line 1/10"))
	}, teatest.WithDuration(5*time.Second))

	tm.Send(press("/"))
	tm.Type("fmt")
	tm.Send(press("enter"))
	tm.Send(press("n"))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("match 2/2"))
	}, teatest.WithDuration(5*time.Second))
	// The first esc clears the search, the second closes.
	tm.Send(press("esc"))
	tm.Send(press("esc"))

	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(host)
	if !ok {
		t.Fatal("final model is not a host")
	}
	if !final.closed || final.pager.Query() != "" {
		t.Errorf("closed %v with query %q; want closed with the search cleared",
			final.closed, final.pager.Query())
	}
	if final.pager.spans == nil {
		t.Error("the highlighted tokens never arrived")
	}
}
