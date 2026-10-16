package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// openedPanes returns the modal of the first pull request, opened in a host
// width columns wide.
func openedPanes(t *testing.T, width int) *panesModal {
	t.Helper()
	h := started(t, newFakeService(), width, 30)
	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the modal")
	}
	return m
}

// TestPanesPress checks the keys that move the focus and the zoom: tab wraps
// from the last pane to the first, a digit focuses its pane and resets which
// of its boxes has the focus, and zoom does nothing where the panes are
// zoomed anyway.
func TestPanesPress(t *testing.T) {
	t.Parallel()
	if panes.WideMin <= 80 || panes.WideMin > 160 {
		t.Fatalf("WideMin is %d: the widths below no longer fall either side of it", panes.WideMin)
	}

	t.Run("tab wraps", func(t *testing.T) {
		t.Parallel()
		m := openedPanes(t, 160)
		start := m.focus
		m.focus = panes.Files
		m.press(keyMsg("tab"))
		if m.focus != panes.Description {
			t.Errorf("tab from Files focused %v, want Description", m.focus)
		}
		m.focus = start
		for range panes.Count {
			m.press(keyMsg("tab"))
		}
		if m.focus != start {
			t.Errorf("%d tabs focused %v, want %v again", panes.Count, m.focus, start)
		}
	})

	t.Run("digit resets the box", func(t *testing.T) {
		t.Parallel()
		m := openedPanes(t, 160)
		m.sub[panes.Merge-1] = panes.Preview
		m.press(keyMsg("3"))
		if m.focus != panes.Merge || m.sub[panes.Merge-1] != panes.List {
			t.Errorf("3 left focus %v and box %v, want Merge and List", m.focus, m.sub[panes.Merge-1])
		}
	})

	t.Run("zoom", func(t *testing.T) {
		t.Parallel()
		m := openedPanes(t, 160)
		m.press(keyMsg("z"))
		if !m.zoom {
			t.Error("z at 160 columns didn't zoom the pane")
		}
		focus := m.focus
		m.press(keyMsg("tab"))
		if m.focus != focus || m.sub[focus-1] != panes.List {
			t.Errorf("tab on a zoomed pane with no preview moved to %v, box %v", m.focus, m.sub[focus-1])
		}
		m.press(keyMsg("z"))
		if m.zoom {
			t.Error("z again didn't unzoom the pane")
		}
	})

	t.Run("zoom is a no-op when narrow", func(t *testing.T) {
		t.Parallel()
		m := openedPanes(t, 80)
		if m.canZoom() {
			t.Fatal("canZoom is true at 80 columns")
		}
		m.press(keyMsg("z"))
		if m.zoom {
			t.Error("z at 80 columns set the zoom, which the panes already have")
		}
	})
}
