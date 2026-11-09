package issues

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// dividedModal opens the first issue at w by 30, with the Development pane
// focused, sharing dividers with a step of 2.
func dividedModal(t *testing.T, w int, zoom bool) (*host, *detailModal, *ui.Dividers) {
	t.Helper()
	h := started(t, blockedSvc(), w, 30)
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	d := &ui.Dividers{Step: 2}
	m.dividers = d
	press(t, h, "4")
	if zoom {
		press(t, h, "z")
	}
	m.SetSize(w, 30)
	return h, m, d
}

// sideCols is the width of the side column of the wide layout.
func sideCols(t *testing.T, m *detailModal) int {
	t.Helper()
	b, ok := m.paneLayout().Box(paneBlockers, panes.Whole)
	if !ok {
		t.Fatal("no Blockers box")
	}
	return b.Rect.W
}

func TestSideDividerMoves(t *testing.T) {
	t.Parallel()
	h, m, _ := dividedModal(t, 160, false)
	if got := sideCols(t, m); got != sideWidth {
		t.Fatalf("start width %d, want %d", got, sideWidth)
	}
	press(t, h, "ctrl+left")
	if got := sideCols(t, m); got != sideWidth+2 {
		t.Errorf("after ctrl+left side is %d, want %d", got, sideWidth+2)
	}
	press(t, h, "ctrl+right", "ctrl+right")
	if got := sideCols(t, m); got != sideWidth-2 {
		t.Errorf("after two ctrl+right side is %d, want %d", got, sideWidth-2)
	}
	press(t, h, "=")
	if got := sideCols(t, m); got != sideWidth {
		t.Errorf("after reset side is %d, want %d", got, sideWidth)
	}
}

func TestSideDividerStopsAtMinimum(t *testing.T) {
	t.Parallel()
	h, m, _ := dividedModal(t, 120, false)
	for range 60 {
		press(t, h, "ctrl+right")
	}
	if got := sideCols(t, m); got != panes.MinWidth {
		t.Errorf("side is %d, want %d", got, panes.MinWidth)
	}
	for range 80 {
		press(t, h, "ctrl+left")
	}
	if got := sideCols(t, m); got != 120-panes.MinWidth {
		t.Errorf("side is %d, want %d", got, 120-panes.MinWidth)
	}
	press(t, h, "ctrl+right")
	if got := sideCols(t, m); got != 120-panes.MinWidth-2 {
		t.Errorf("a press at the limit piled up: side is %d", got)
	}
}

func TestSideDividerSettlesOnResize(t *testing.T) {
	t.Parallel()
	h, m, d := dividedModal(t, 160, false)
	for range 80 {
		press(t, h, "ctrl+left")
	}
	m.SetSize(130, 30)
	if got := sideCols(t, m); got != 130-panes.MinWidth {
		t.Errorf("side is %d after resize, want %d", got, 130-panes.MinWidth)
	}
	if !d.Side.Moved() {
		t.Error("the offset was lost")
	}
}

func TestZoomedSplitFollowsList(t *testing.T) {
	t.Parallel()
	h, m, d := dividedModal(t, 160, true)
	list := func() int {
		b, ok := m.paneLayout().Box(paneDevelopment, panes.List)
		if !ok {
			t.Fatal("the zoomed Development pane has no list")
		}
		return b.Rect.W
	}
	start := list()
	press(t, h, "ctrl+right")
	if got := list(); got != start+2 {
		t.Errorf("list is %d, want %d", got, start+2)
	}
	if d.Side.Moved() {
		t.Error("the zoomed press moved the side column")
	}
	press(t, h, "=")
	if got := list(); got != start {
		t.Errorf("list is %d after reset, want %d", got, start)
	}
}

func TestDividerDoesNothingWithoutADivider(t *testing.T) {
	t.Parallel()
	h, narrow, d := dividedModal(t, 100, false)
	// A narrow modal shows one pane; the Conversation has no preview.
	press(t, h, "2", "ctrl+left")
	if d.Moved() || narrow.canDivide() {
		t.Error("a narrow modal moved a divider")
	}
	h, m, d := dividedModal(t, 160, false)
	press(t, h, "2", "z")
	press(t, h, "ctrl+left")
	if d.Moved() || m.canDivide() {
		t.Error("a zoomed pane with no preview moved a divider")
	}
	_, alone, _ := dividedModal(t, 160, false)
	alone.dividers = nil
	if alone.canDivide() {
		t.Error("a modal with no dividers can divide")
	}
	if got := sideCols(t, alone); got != sideWidth {
		t.Errorf("nil dividers changed the layout: side %d", got)
	}
}

func TestSideDividerSharedByName(t *testing.T) {
	t.Parallel()
	d := &ui.Dividers{Step: 2}
	d.Side.Move(4)
	if got := sideSplit(d, 160); got != sideWidth-4 {
		t.Errorf("side split %d, want %d", got, sideWidth-4)
	}
	if got := sideSplit(nil, 160); got != sideWidth {
		t.Errorf("nil side split %d, want %d", got, sideWidth)
	}
}

func TestDividerHelpRowsFollowTheLayout(t *testing.T) {
	t.Parallel()
	h, m, _ := dividedModal(t, 160, false)
	enabled := func() bool {
		for _, l := range m.KeyLayers() {
			for _, b := range l.Bindings {
				if b.Help().Desc == "divider left" {
					return b.Enabled()
				}
			}
		}
		t.Fatal("no divider row")
		return false
	}
	if !enabled() {
		t.Error("the divider row is disabled in the wide layout")
	}
	m.SetSize(100, 30)
	press(t, h, "2")
	if enabled() {
		t.Error("the divider row is enabled in a narrow layout")
	}
}
