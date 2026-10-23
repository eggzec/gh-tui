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
		if m.canZoom() != tt.can {
			t.Errorf("canZoom at %d columns = %v, want %v", tt.width, m.canZoom(), tt.can)
		}
		h.keys("z")
		if m.zoom && !tt.can {
			t.Errorf("z zoomed the modal at %d columns", tt.width)
		}
		if tt.can && !m.zoom {
			t.Errorf("z didn't zoom the modal at %d columns", tt.width)
		}
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
	if m.zoom {
		t.Error("z again didn't unzoom")
	}
}

// The digits focus the runs and the annotations, and tab goes through the
// runs, the jobs and the annotations.
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
	h.keys("tab", "tab")
	if !m.notes {
		t.Error("tab twice from the runs isn't on the annotations")
	}
	h.keys("tab")
	if m.notes || m.focus != runsPane {
		t.Error("tab from the annotations doesn't wrap to the runs")
	}
}

// The annotations tell what to do when there is nothing to show.
func TestAnnotationsEmptyState(t *testing.T) {
	m, _ := newModal(t, newFake(), wideW, wideH)
	if s := screen(m); !strings.Contains(s, "No annotations here.") {
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
