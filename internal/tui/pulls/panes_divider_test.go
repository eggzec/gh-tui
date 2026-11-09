package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// dividedPanes returns an opened modal that shares dividers with a step of 2.
func dividedPanes(t *testing.T, width int) (*panesModal, *ui.Dividers) {
	t.Helper()
	m := openedPanes(t, width)
	d := &ui.Dividers{Step: 2}
	m.list.dividers = d
	m.SetSize(width, 30)
	return m, d
}

// sideOf is the width of the side column of the wide layout.
func sideWide(t *testing.T, m *panesModal) int {
	t.Helper()
	b, ok := m.layout().Box(paneMerge, panes.Whole)
	if !ok {
		t.Fatal("no Merge box")
	}
	return b.Rect.W
}

func TestSideDividerMoves(t *testing.T) {
	t.Parallel()
	m, _ := dividedPanes(t, 160)
	if got := sideWide(t, m); got != sideWidth {
		t.Fatalf("start width %d, want %d", got, sideWidth)
	}
	m.press(keyMsg("ctrl+left"))
	if got := sideWide(t, m); got != sideWidth+2 {
		t.Errorf("after ctrl+left side is %d, want %d", got, sideWidth+2)
	}
	m.press(keyMsg("ctrl+right"))
	m.press(keyMsg("ctrl+right"))
	if got := sideWide(t, m); got != sideWidth-2 {
		t.Errorf("after two ctrl+right side is %d, want %d", got, sideWidth-2)
	}
	m.press(keyMsg("="))
	if got := sideWide(t, m); got != sideWidth {
		t.Errorf("after reset side is %d, want %d", got, sideWidth)
	}
}

func TestSideDividerStopsAtMinimum(t *testing.T) {
	t.Parallel()
	m, _ := dividedPanes(t, 120)
	for range 60 {
		m.press(keyMsg("ctrl+right"))
	}
	if got := sideWide(t, m); got != panes.MinWidth {
		t.Errorf("side is %d, want %d", got, panes.MinWidth)
	}
	for range 80 {
		m.press(keyMsg("ctrl+left"))
	}
	if got := sideWide(t, m); got != 120-panes.MinWidth {
		t.Errorf("side is %d, want %d", got, 120-panes.MinWidth)
	}
	m.press(keyMsg("ctrl+right"))
	if got := sideWide(t, m); got != 120-panes.MinWidth-2 {
		t.Errorf("a press at the limit piled up: side is %d", got)
	}
}

func TestSideDividerSettlesOnResize(t *testing.T) {
	t.Parallel()
	m, d := dividedPanes(t, 160)
	for range 80 {
		m.press(keyMsg("ctrl+left"))
	}
	m.SetSize(130, 30)
	if got := sideWide(t, m); got != 130-panes.MinWidth {
		t.Errorf("side is %d after resize, want %d", got, 130-panes.MinWidth)
	}
	if !d.Side.Moved() {
		t.Error("the offset was lost")
	}
}

func TestZoomedSplitFollowsList(t *testing.T) {
	t.Parallel()
	m, d := dividedPanes(t, 160)
	m.nav.SetFocus(paneFiles)
	m.press(keyMsg("z"))
	list := func() int {
		b, ok := m.layout().Box(paneFiles, panes.List)
		if !ok {
			t.Fatal("the zoomed Files pane has no list")
		}
		return b.Rect.W
	}
	start := list()
	m.press(keyMsg("ctrl+right"))
	if got := list(); got != start+2 {
		t.Errorf("list is %d, want %d", got, start+2)
	}
	if d.Side.Moved() {
		t.Error("the zoomed press moved the side column")
	}
	m.press(keyMsg("="))
	if got := list(); got != start {
		t.Errorf("list is %d after reset, want %d", got, start)
	}
}

func TestDividerDoesNothingWithoutADivider(t *testing.T) {
	t.Parallel()
	narrow, d := dividedPanes(t, 100)
	narrow.press(keyMsg("ctrl+left"))
	if d.Moved() || narrow.canDivide() {
		t.Error("a narrow modal moved a divider")
	}
	m, d := dividedPanes(t, 160)
	m.nav.SetFocus(paneConversation)
	m.press(keyMsg("z"))
	m.press(keyMsg("ctrl+left"))
	if d.Moved() || m.canDivide() {
		t.Error("a zoomed pane with no preview moved a divider")
	}
	alone := openedPanes(t, 160)
	alone.SetSize(160, 30)
	alone.press(keyMsg("ctrl+left"))
	if alone.canDivide() {
		t.Error("a modal with no dividers can divide")
	}
	if got := sideWide(t, alone); got != 160-(160-sideWidth) {
		t.Errorf("nil dividers changed the layout: side %d", got)
	}
}

func TestDividerHelpRowsFollowTheLayout(t *testing.T) {
	t.Parallel()
	m, _ := dividedPanes(t, 160)
	enabled := func(m *panesModal) bool {
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
	if !enabled(m) {
		t.Error("the divider row is disabled in the wide layout")
	}
	m.SetSize(100, 30)
	if enabled(m) {
		t.Error("the divider row is enabled in a narrow layout")
	}
}
