package actions

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// dividedModal is a modal w cells wide that shares a divider set.
func dividedModal(t *testing.T, w int) (*Modal, *host, *ui.Dividers) {
	t.Helper()
	m, h := newModal(t, newFake(), w, 30)
	d := &ui.Dividers{Step: 2}
	m.SetDividers(d)
	return m, h, d
}

// runsCols is the width of the runs pane.
func runsCols(t *testing.T, m *Modal) int {
	t.Helper()
	aw, ah := m.area()
	b, ok := paneLayout(aw, ah, paneRuns, m.zoom.Zoomed(), m.dividers).Box(paneRuns, panes.Whole)
	if !ok {
		t.Fatal("no runs box")
	}
	return b.Rect.W
}

func TestRunsDividerMoves(t *testing.T) {
	t.Parallel()
	m, h, d := dividedModal(t, 120)
	base := runsCols(t, m)
	h.keys("ctrl+right")
	if got := runsCols(t, m); got != base+2 {
		t.Errorf("runs %d after ctrl+right, want %d", got, base+2)
	}
	h.keys("ctrl+left", "ctrl+left")
	if got := runsCols(t, m); got != base-2 {
		t.Errorf("runs %d after two ctrl+left, want %d", got, base-2)
	}
	h.keys("=")
	if got := runsCols(t, m); got != base || d.List.Moved() {
		t.Errorf("runs %d after reset, want %d", got, base)
	}
}

func TestRunsDividerStopsAtMinimum(t *testing.T) {
	t.Parallel()
	m, h, _ := dividedModal(t, 120)
	for range 80 {
		h.keys("ctrl+left")
	}
	if got := runsCols(t, m); got != panes.MinWidth {
		t.Errorf("runs %d at the left limit, want %d", got, panes.MinWidth)
	}
	h.keys("ctrl+right")
	if got := runsCols(t, m); got != panes.MinWidth+2 {
		t.Errorf("runs %d after one press back, want %d", got, panes.MinWidth+2)
	}
	for range 80 {
		h.keys("ctrl+right")
	}
	aw, _ := m.area()
	if got := aw - runsCols(t, m); got != panes.MinWidth {
		t.Errorf("annotations %d at the right limit, want %d", got, panes.MinWidth)
	}
}

func TestRunsDividerSettlesOnResize(t *testing.T) {
	t.Parallel()
	m, h, d := dividedModal(t, 130)
	for range 80 {
		h.keys("ctrl+right")
	}
	m.SetSize(110, 30)
	aw, _ := m.area()
	if got := aw - runsCols(t, m); got < panes.MinWidth {
		t.Errorf("annotations %d after a resize, want at least %d", got, panes.MinWidth)
	}
	if !d.List.Moved() {
		t.Error("the offset was lost on resize")
	}
}

func TestRunsDividerSharedWithSearch(t *testing.T) {
	t.Parallel()
	m, _, d := dividedModal(t, 120)
	base := runsCols(t, m)
	d.List.Move(4)
	m.layout()
	if got := runsCols(t, m); got != base+4 {
		t.Errorf("runs %d after the shared list divider moved, want %d", got, base+4)
	}
}

func TestRunsDividerDoesNothingWhereNoneShows(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		w    int
		zoom bool
		nil  bool
	}{"narrow": {w: 90}, "zoomed": {w: 120, zoom: true}, "no dividers": {w: 120, nil: true}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, h, d := dividedModal(t, tc.w)
			if tc.nil {
				m.SetDividers(nil)
			}
			m.zoom.SetZoom(tc.zoom)
			h.keys("ctrl+right")
			if d.List.Moved() {
				t.Error("the divider moved")
			}
			for _, l := range m.KeyLayers() {
				for _, b := range l.Bindings {
					if b.Help().Desc == "divider right" && b.Enabled() {
						t.Error("the divider key is enabled in help")
					}
				}
			}
		})
	}
}

func TestRunsDividerHelpEnabledWhenWide(t *testing.T) {
	t.Parallel()
	m, _, _ := dividedModal(t, 120)
	for _, l := range m.KeyLayers() {
		for _, b := range l.Bindings {
			if b.Help().Desc == "divider right" {
				if !b.Enabled() {
					t.Error("the divider key is disabled in help while both panes show")
				}
				return
			}
		}
	}
	t.Error("help has no divider key")
}
