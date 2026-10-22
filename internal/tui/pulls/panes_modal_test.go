package pulls

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

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

// TestPullModalZoomBoundary checks when the zoom is forced: by a narrow
// terminal, or by an area under wideMin, whatever the terminal is.
func TestPullModalZoomBoundary(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		narrow bool
		width  int
		forced bool
	}{
		{"wide terminal, wide area", false, wideMin, false},
		{"wide terminal, area one under", false, wideMin - 1, true},
		{"narrow terminal, wide area", true, 160, true},
		{"narrow terminal, narrow area", true, 80, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := openedPanes(t, 160)
			m.SetNarrow(tt.narrow)
			m.SetSize(tt.width, 30)
			if got := m.layout().Zoomed; got != tt.forced {
				t.Errorf("zoomed = %v, want %v", got, tt.forced)
			}
			if m.zoom.CanZoom() == tt.forced {
				t.Errorf("canZoom = %v with a forced zoom of %v", m.zoom.CanZoom(), tt.forced)
			}
		})
	}
}

// TestPullModalSmallAreaForcesZoom checks that a small modal in a wide
// terminal is zoomed and the zoom key does nothing there.
func TestPullModalSmallAreaForcesZoom(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, 160)
	m.SetNarrow(false)
	m.SetSize(100, 30)
	if !m.layout().Zoomed {
		t.Fatal("a 100 column area isn't zoomed")
	}
	m.press(keyMsg("z"))
	if m.zoom.User() {
		t.Error("z set the user's zoom in a small area")
	}
}

// TestPullModalUserZoomSurvivesNarrow checks that the user's zoom stays
// while a narrow terminal forces one, and shows again after.
func TestPullModalUserZoomSurvivesNarrow(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, 160)
	m.press(keyMsg("z"))
	if !m.zoom.User() {
		t.Fatal("z didn't zoom at 160 columns")
	}
	m.SetNarrow(true)
	m.SetSize(160, 30)
	m.press(keyMsg("backspace"))
	if !m.zoom.User() || !m.layout().Zoomed {
		t.Error("backspace changed the user's zoom while the terminal is narrow")
	}
	m.SetNarrow(false)
	m.SetSize(160, 30)
	if !m.zoom.User() || !m.layout().Zoomed {
		t.Error("the user's zoom was lost after the narrow phase")
	}
	m.press(keyMsg("backspace"))
	if m.zoom.User() || m.layout().Zoomed {
		t.Error("backspace didn't undo the user's zoom")
	}
}

// TestPullModalNameAloneOnTopEdge checks that a zoomed pane names itself in
// the top edge without its digit, which the strip of the bottom edge has.
func TestPullModalNameAloneOnTopEdge(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, 100)
	m.SetSize(100, 30)
	l := m.layout()
	if !l.Zoomed {
		t.Fatal("a 100 column area isn't zoomed")
	}
	top, _, _ := strings.Cut(ansi.Strip(m.panes(l)), "\n")
	name := paneName(m.focus)
	if !strings.Contains(top, name) || strings.Contains(top, paneTitle(m.focus)) {
		t.Errorf("top edge %q should name %q without its digit", top, name)
	}
}

// TestPanesPress checks the keys that move the focus and the zoom: tab wraps
// from the last pane to the first, a digit focuses its pane and resets which
// of its boxes has the focus, and zoom does nothing where the panes are
// zoomed anyway.
func TestPanesPress(t *testing.T) {
	t.Parallel()
	if wideMin <= 80 || wideMin > 160 {
		t.Fatalf("WideMin is %d: the widths below no longer fall either side of it", wideMin)
	}

	t.Run("tab wraps", func(t *testing.T) {
		t.Parallel()
		m := openedPanes(t, 160)
		start := m.focus
		m.focus = paneFiles
		m.press(keyMsg("tab"))
		if m.focus != paneDescription {
			t.Errorf("tab from Files focused %v, want Description", m.focus)
		}
		m.focus = start
		for range paneCount {
			m.press(keyMsg("tab"))
		}
		if m.focus != start {
			t.Errorf("%d tabs focused %v, want %v again", paneCount, m.focus, start)
		}
	})

	t.Run("digit resets the box", func(t *testing.T) {
		t.Parallel()
		m := openedPanes(t, 160)
		m.sub[paneMerge-1] = panes.Preview
		m.press(keyMsg("3"))
		if m.focus != paneMerge || m.sub[paneMerge-1] != panes.List {
			t.Errorf("3 left focus %v and box %v, want Merge and List", m.focus, m.sub[paneMerge-1])
		}
	})

	t.Run("zoom", func(t *testing.T) {
		t.Parallel()
		m := openedPanes(t, 160)
		m.press(keyMsg("z"))
		if !m.zoom.Zoomed() {
			t.Error("z at 160 columns didn't zoom the pane")
		}
		focus := m.focus
		m.press(keyMsg("tab"))
		if m.focus != focus || m.sub[focus-1] != panes.List {
			t.Errorf("tab on a zoomed pane with no preview moved to %v, box %v", m.focus, m.sub[focus-1])
		}
		m.press(keyMsg("z"))
		if m.zoom.Zoomed() {
			t.Error("z again didn't unzoom the pane")
		}
	})

	t.Run("zoom is a no-op when narrow", func(t *testing.T) {
		t.Parallel()
		m := openedPanes(t, 80)
		if m.zoom.CanZoom() {
			t.Fatal("canZoom is true at 80 columns")
		}
		m.press(keyMsg("z"))
		if m.zoom.User() {
			t.Error("z at 80 columns set the zoom, which the panes already have")
		}
	})
}
