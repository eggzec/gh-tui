package issues

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// openDev opens the first issue at w by 30 with the Development pane focused
// and the zoom as zoom says, where the width allows it.
func openDev(t *testing.T, w int, zoom bool) *detailModal {
	t.Helper()
	h := started(t, blockedSvc(), w, 30)
	press(t, h, "down", "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	press(t, h, "4")
	if m.zoom.Zoomed() != zoom {
		press(t, h, "z")
	}
	return m
}

func TestZoomedSplitPaneDrawnOnce(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		w      int
		zoom   bool
		zoomed bool
	}{
		{99, true, true},
		{100, true, true},
		{119, true, true},
		{120, true, true},
		{120, false, false},
	} {
		t.Run(fmt.Sprintf("%d zoom %v", tc.w, tc.zoom), func(t *testing.T) {
			t.Parallel()
			m := openDev(t, tc.w, tc.zoom)
			l := m.paneLayout()
			if l.Zoomed != tc.zoomed {
				t.Fatalf("zoomed = %v, want %v", l.Zoomed, tc.zoomed)
			}
			n := 0
			for _, b := range l.Boxes {
				if b.Pane == 4 && b.Role != panes.Preview {
					n++
				}
			}
			if n != 1 {
				t.Errorf("%d boxes draw the pane, want 1", n)
			}
			v := ansi.Strip(m.View())
			assertFits(t, m.View(), tc.w, 30)
			title := "Development"
			if !tc.zoomed {
				title = "[4] Development"
			}
			if got := strings.Count(v, "┤ "+title) + strings.Count(v, " "+title+" "); got < 1 {
				t.Errorf("the title %q is missing:\n%s", title, v)
			}
			strip := strings.Count(v, "1 Desc")
			want := 0
			if tc.zoomed {
				want = 1
			}
			if strip != want {
				t.Errorf("the hint strip shows %d times, want %d:\n%s", strip, want, v)
			}
			if !tc.zoomed {
				return
			}
			if got := strings.Count(v, "╭─ Development"); got != 1 {
				t.Errorf("%d boxes are titled Development, want 1:\n%s", got, v)
			}
		})
	}
}

func TestPreviewFocusDropsOnResize(t *testing.T) {
	t.Parallel()
	m := openDev(t, 110, true)
	step := func() { m.nav.Step(m.paneLayout(), 1) }
	step()
	if !m.nav.InPreview(m.paneLayout()) {
		t.Fatal("the step didn't reach the preview at 110")
	}
	m.SetSize(90, 30)
	if m.nav.Sub(4) != panes.List {
		t.Error("the focus stayed in the preview after the width fell under 100")
	}
	m.SetSize(110, 30)
	if m.nav.InPreview(m.paneLayout()) {
		t.Error("the preview took the focus back on a wider window")
	}
}

func TestZoomedSplitPaneView(t *testing.T) {
	t.Parallel()
	for _, w := range []int{80, 110} {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			t.Parallel()
			m := openDev(t, w, true)
			v := m.View()
			assertFits(t, v, w, 30)
			golden.RequireEqual(t, v)
		})
	}
}
