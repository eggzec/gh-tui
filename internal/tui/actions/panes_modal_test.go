package actions

import (
	"strings"
	"testing"
)

// The zoom is available from wideMin columns on, and only there.
func TestZoomBoundary(t *testing.T) {
	for _, tt := range []struct {
		width int
		can   bool
	}{{wideMin, true}, {wideMin - 1, false}, {80, false}} {
		m, h := newModal(t, newFake(), tt.width, wideH)
		if m.zoom.CanZoom() != tt.can {
			t.Errorf("CanZoom at %d columns = %v, want %v", tt.width, m.zoom.CanZoom(), tt.can)
		}
		h.keys("z")
		if m.zoom.User() != tt.can {
			t.Errorf("after z at %d columns the user's zoom = %v, want %v", tt.width, m.zoom.User(), tt.can)
		}
	}
}

// A narrow terminal forces the zoom, and growing again brings back the
// user's choice; only the zoom key undoes it, never back.
func TestZoomNarrowAndBack(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	h.keys("z")
	m.SetNarrow(true)
	m.SetSize(wideW, wideH)
	h.keys("z")
	if !m.zoom.Zoomed() || !m.zoom.User() || m.zoom.CanZoom() {
		t.Errorf("forced: zoomed %v, user %v, can %v", m.zoom.Zoomed(), m.zoom.User(), m.zoom.CanZoom())
	}
	m.SetNarrow(false)
	m.SetSize(wideW, wideH)
	if !m.zoom.Zoomed() || !m.zoom.CanZoom() {
		t.Error("growing again lost the user's zoom")
	}
	h.keys("backspace")
	if !m.zoom.Zoomed() {
		t.Error("back undid the user's zoom: only the zoom key does")
	}
	h.keys("z")
	if m.zoom.Zoomed() {
		t.Error("z didn't undo the user's zoom")
	}
	m.SetNarrow(true)
	m.SetSize(wideW, wideH)
	m.SetNarrow(false)
	m.SetSize(wideW, wideH)
	if m.zoom.Zoomed() {
		t.Error("a forced zoom stayed after the terminal grew")
	}
}

// Side by side, both boxes show; zoomed, the focused one shows alone with the
// strip of pane names under it.
func TestZoomShowsOneBox(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	s := screen(m)
	if !strings.Contains(s, "Runs") || !strings.Contains(s, "Annotations") {
		t.Errorf("the wide modal lacks a box:\n%s", s)
	}
	h.keys("z")
	s = screen(m)
	if strings.Contains(s, "Workflow CI") {
		t.Errorf("the zoomed runs still show the annotations:\n%s", s)
	}
	if !strings.Contains(lastLines(s, 2), "Annotations") {
		t.Errorf("the zoomed modal has no strip of pane names:\n%s", s)
	}
	assertFits(t, m.View(), wideW, wideH)
	h.keys("z")
	if m.zoom.Zoomed() {
		t.Error("z again didn't unzoom")
	}
}

// The digits focus the runs and the annotations, and tab goes through the
// runs and the annotations.
func TestDigitsAndTabReachAnnotations(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	h.keys("2")
	if !m.notes {
		t.Fatal("2 didn't focus the annotations")
	}
	h.keys("1")
	if m.notes || m.focus != runsPane {
		t.Fatal("1 didn't focus the runs")
	}
	h.keys("tab")
	if !m.notes {
		t.Error("tab from the runs isn't on the annotations")
	}
	h.keys("tab")
	if m.notes || m.focus != runsPane {
		t.Error("tab from the annotations doesn't wrap to the runs")
	}
}

// The annotations tell what to do when there is nothing to show.
func TestAnnotationsEmptyState(t *testing.T) {
	f := newFake()
	clear(f.notes)
	m, _ := newModal(t, f, wideW, wideH)
	if s := screen(m); !strings.Contains(s, "No annotations on this run.") {
		t.Errorf("the annotations lack their empty state:\n%s", s)
	}
}

// The modal fits its area at the widths the layouts change at.
func TestPanesFitAt80And120(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {99, 24}, {100, 24}, {120, 40}} {
		for _, keys := range [][]string{nil, {"z"}, {"tab"}, {"2"}} {
			m, h := newModal(t, newFake(), size[0], size[1])
			h.keys(keys...)
			assertFits(t, m.View(), size[0], size[1])
		}
	}
}

// lastLines is the last n lines of s.
func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	return strings.Join(lines[max(len(lines)-n, 0):], "\n")
}
