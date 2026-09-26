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
	// second records that n reached the second match of "fmt". The
	// output can't tell: the renderer redraws only the changed digit.
	second bool
	// highlighted is closed once the highlighted tokens arrive, which
	// they do in a command that the keys may beat.
	highlighted chan struct{}
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
	if h.pager.Query() == "fmt" && h.pager.search.cur == 1 {
		h.second = true
	}
	if h.highlighted != nil && h.pager.spans != nil {
		close(h.highlighted)
		h.highlighted = nil
	}
	return h, cmd
}

func (h host) View() tea.View { return tea.NewView(h.pager.View()) }

func TestProgram(t *testing.T) {
	p := New()
	highlight := p.SetContent("main.go", goSource)
	p.Focus()
	highlighted := make(chan struct{})
	tm := teatest.NewTestModel(t, host{pager: p, highlight: highlight, highlighted: highlighted}, teatest.WithInitialTermSize(60, 12))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("line 1/10"))
	}, teatest.WithDuration(5*time.Second))
	select {
	case <-highlighted:
	case <-time.After(5 * time.Second):
		t.Fatal("the highlighted tokens never arrived")
	}

	tm.Send(press("/"))
	tm.Type("fmt")
	tm.Send(press("enter"))
	tm.Send(press("n"))
	// The first esc clears the search, the second closes.
	tm.Send(press("esc"))
	tm.Send(press("esc"))

	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(host)
	if !ok {
		t.Fatal("final model is not a host")
	}
	if !final.second {
		t.Error("n didn't go to the second match")
	}
	if !final.closed || final.pager.Query() != "" {
		t.Errorf("closed %v with query %q; want closed with the search cleared",
			final.closed, final.pager.Query())
	}
}
