package pulls

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// stubOverlay is an overlay of a kind that has a transient state to clear.
type stubOverlay struct {
	k         panes.OverlayKind
	name      string
	rows      int
	transient bool
	done      bool
	got       []tea.Msg
}

func (o *stubOverlay) title() string           { return o.name }
func (o *stubOverlay) kind() panes.OverlayKind { return o.k }
func (o *stubOverlay) items() int              { return o.rows }
func (o *stubOverlay) text() int               { return 0 }
func (o *stubOverlay) closed() bool            { return o.done }
func (o *stubOverlay) view(w, h int) string {
	return strings.Repeat(strings.Repeat("x", w)+"\n", max(h-1, 0)) + strings.Repeat("x", w)
}
func (o *stubOverlay) layers() []keyhelp.Layer {
	return []keyhelp.Layer{{Source: "stub", Context: "stub"}}
}
func (o *stubOverlay) update(msg tea.Msg) tea.Cmd {
	o.got = append(o.got, msg)
	return nil
}
func (o *stubOverlay) clear() bool {
	was := o.transient
	o.transient = false
	return was
}

// sizedPanes returns the modal of openedPanes, sized w by h.
func sizedPanes(t *testing.T, w, h int) *panesModal {
	t.Helper()
	m := openedPanes(t, w)
	m.SetSize(w, h)
	return m
}

// TestConfirmOverlayKeepsThePanes checks that a question opened over the
// panes and answered leaves the focus, zoom and sub focus as they were, and
// that no key reaches the panes while it is open.
func TestConfirmOverlayKeepsThePanes(t *testing.T) {
	t.Parallel()
	m := sizedPanes(t, 160, 40)
	m.focus = panes.Merge
	m.zoom.SetZoom(true)
	m.sub[panes.Merge-1] = panes.Preview
	yes := 0
	m.ask(ui.Confirm{Question: "Close #1?", Run: func() tea.Cmd { yes++; return nil }})

	for _, k := range []string{"tab", "shift+tab", "3", "z", "enter", "j"} {
		m.Update(keyMsg(k))
		if m.focus != panes.Merge || !m.zoom.Zoomed() || m.sub[panes.Merge-1] != panes.Preview || m.top() == nil {
			t.Fatalf("%s under a question changed the modal: focus %v zoom %v sub %v open %v", k, m.focus, m.zoom.Zoomed(), m.sub, m.top() != nil)
		}
	}
	if yes != 0 {
		t.Error("a key other than the answer ran the change")
	}
	m.Update(keyMsg("n"))
	if m.top() != nil {
		t.Fatal("n left the question open")
	}
	if m.focus != panes.Merge || !m.zoom.Zoomed() || m.sub[panes.Merge-1] != panes.Preview {
		t.Errorf("after the question: focus %v zoom %v sub %v", m.focus, m.zoom.Zoomed(), m.sub)
	}
	m.ask(ui.Confirm{Question: "Close #1?", Run: func() tea.Cmd { yes++; return nil }})
	m.Update(keyMsg("y"))
	if yes != 1 || m.top() != nil {
		t.Errorf("y ran the change %d times, open %v", yes, m.top() != nil)
	}
}

// TestConfirmQuitClosesTheModal checks that the quit key closes the whole
// modal from under a question, which takes every key, without answering it.
func TestConfirmQuitClosesTheModal(t *testing.T) {
	t.Parallel()
	m := sizedPanes(t, 160, 40)
	ran := false
	m.ask(ui.Confirm{Question: "Close #1?", Run: func() tea.Cmd { ran = true; return nil }})
	m.Update(keyMsg("q"))
	if !m.closed || ran {
		t.Errorf("q under a question: modal closed %v, change ran %v", m.closed, ran)
	}
}

// TestOverlayLayers checks that an open overlay's keys are the only layers.
func TestOverlayLayers(t *testing.T) {
	t.Parallel()
	m := sizedPanes(t, 160, 40)
	if n := len(m.KeyLayers()); n < 2 {
		t.Fatalf("the modal has %d layers", n)
	}
	m.ask(ui.Confirm{Question: "Merge?"})
	got := m.KeyLayers()
	if len(got) != 1 || got[0].Context != "confirm" {
		t.Errorf("layers under a question: %d, first %q; want the confirm's alone", len(got), got[0].Context)
	}
	if got := m.KeyContext(); got != ctxModal {
		t.Errorf("KeyContext = %q, want %q", got, ctxModal)
	}
}

// TestViewerOverlayClosing checks esc, backspace and quit in a viewer: esc
// clears the transient state first and then closes the viewer alone, backspace
// closes the viewer, and quit closes the modal.
func TestViewerOverlayClosing(t *testing.T) {
	t.Parallel()
	open := func() (*panesModal, *stubOverlay) {
		m := sizedPanes(t, 160, 40)
		v := &stubOverlay{k: panes.Viewer, name: "Viewer", transient: true}
		m.open(v)
		return m, v
	}

	m, v := open()
	if _, ok := m.Act(ui.ActDismiss); !ok || m.top() == nil || v.transient {
		t.Fatalf("first dismiss: open %v, transient %v", m.top() != nil, v.transient)
	}
	if _, ok := m.Act(ui.ActDismiss); !ok || m.top() != nil || m.closed {
		t.Fatalf("second dismiss: open %v, modal closed %v", m.top() != nil, m.closed)
	}
	if _, ok := m.Act(ui.ActDismiss); !ok || !m.closed {
		t.Error("dismiss with nothing open didn't close the modal")
	}

	m, _ = open()
	if _, ok := m.Act(ui.ActBack); !ok || m.top() != nil || m.closed {
		t.Errorf("back: open %v, modal closed %v", m.top() != nil, m.closed)
	}

	m, _ = open()
	if _, ok := m.Act(ui.ActQuit); !ok || !m.closed {
		t.Error("quit didn't close the modal from a viewer")
	}

	// A picker closes with its own keys, which dismiss leaves to it.
	m = sizedPanes(t, 160, 40)
	m.open(&stubOverlay{k: panes.Picker, name: "Picker", rows: 3})
	if _, ok := m.Act(ui.ActDismiss); ok || m.top() == nil {
		t.Error("dismiss was taken from a picker")
	}
	if _, ok := m.Act(ui.ActBack); !ok || m.top() == nil {
		t.Error("back closed a picker, or reached the app")
	}
	if _, ok := m.Act(ui.ActMaximize); ok {
		t.Error("maximize was taken by a picker")
	}
}

// TestMaximizeViewer checks that the maximize intent flips a viewer between
// its inset and the whole area, and that without one the app is left to
// maximize the modal.
func TestMaximizeViewer(t *testing.T) {
	t.Parallel()
	m := sizedPanes(t, 160, 40)
	if _, ok := m.Act(ui.ActMaximize); ok {
		t.Fatal("maximize taken with no overlay open")
	}
	m.open(&stubOverlay{k: panes.Viewer, name: "Viewer"})
	inset := m.placed()
	if want := panes.OverlayRect(panes.Overlay{Kind: panes.Viewer}, 160, 38); inset != want {
		t.Fatalf("viewer at %v, want %v", inset, want)
	}
	if _, ok := m.Act(ui.ActMaximize); !ok {
		t.Fatal("maximize not taken by a viewer")
	}
	if got, want := m.placed(), panes.OverlayRect(panes.Overlay{Kind: panes.Viewer, Maximized: true}, 160, 38); got != want {
		t.Errorf("maximized viewer at %v, want %v", got, want)
	}
	m.Act(ui.ActMaximize)
	if m.placed() != inset {
		t.Errorf("viewer after a second maximize at %v, want %v", m.placed(), inset)
	}
}

// TestOverlayPlacement checks where an overlay is drawn: at the rectangle
// that panes.OverlayRect gives for the area below the header, over dimmed
// panes, with the header and a footer of its own keys.
func TestOverlayPlacement(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{120, 40}, {80, 24}} {
		for _, tc := range []struct {
			name string
			open func(m *panesModal)
			want panes.Overlay
		}{
			{"Viewer", func(m *panesModal) { m.open(&stubOverlay{k: panes.Viewer, name: "Viewer"}) }, panes.Overlay{Kind: panes.Viewer}},
			{"Picker", func(m *panesModal) { m.open(&stubOverlay{k: panes.Picker, name: "Picker", rows: 5}) }, panes.Overlay{Kind: panes.Picker, Items: 5}},
			{"Confirm", func(m *panesModal) { m.ask(ui.Confirm{Question: "Merge?"}) }, panes.Overlay{Kind: panes.Confirm, Text: len("Merge?")}},
		} {
			w, h := size[0], size[1]
			m := sizedPanes(t, w, h)
			tc.open(m)
			r := panes.OverlayRect(tc.want, w, h-2)
			if got := m.placed(); got != r {
				t.Errorf("%s at %dx%d: placed %v, want %v", tc.name, w, h, got, r)
			}
			lines := strings.Split(m.View(), "\n")
			if len(lines) != h {
				t.Fatalf("%s at %dx%d: %d lines", tc.name, w, h, len(lines))
			}
			for i, l := range lines {
				if got := ansi.StringWidth(l); got != w {
					t.Errorf("%s at %dx%d: line %d is %d cells wide", tc.name, w, h, i, got)
				}
			}
			if !strings.Contains(ansi.Strip(lines[1+r.Y]), tc.name) {
				t.Errorf("%s at %dx%d: no title on line %d, the top of the box (header offset 1): %q", tc.name, w, h, 1+r.Y, ansi.Strip(lines[1+r.Y]))
			}
			if !strings.Contains(ansi.Strip(lines[0]), "#") {
				t.Errorf("%s at %dx%d: the header is gone: %q", tc.name, w, h, lines[0])
			}
		}
	}
}

// TestOverlayFooter checks that the footer names the keys of the overlay only.
func TestOverlayFooter(t *testing.T) {
	t.Parallel()
	m := sizedPanes(t, 120, 30)
	m.ask(ui.Confirm{Question: "Merge?"})
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "yes") || !strings.Contains(last, "no") || strings.Contains(last, "zoom") || strings.Contains(last, "focus") {
		t.Errorf("footer under a question: %q", last)
	}
}
