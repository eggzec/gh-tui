package actions

import (
	"strings"
	"testing"
)

// The details of a run say when it started and how long it took, by its jobs.
func TestRunDetailsStartedAndDuration(t *testing.T) {
	t.Parallel()
	m, h := newModal(t, newFake(), 80, 24)
	h.keys("2")
	s := strings.Join(strings.Fields(screen(m)), " ")
	if !strings.Contains(s, "Started 2h") || !strings.Contains(s, "Duration 3m 2s") {
		t.Errorf("the details of the failed run:\n%s", screen(m))
	}

	f := newFake()
	f.setJobs(failedRun, nil)
	m, h = newModal(t, f, 80, 24)
	h.keys("2")
	s = strings.Join(strings.Fields(screen(m)), " ")
	if !strings.Contains(s, "Duration -") {
		t.Errorf("the details of a run with no jobs:\n%s", screen(m))
	}
}

// Moving the focus from the jobs to the annotations leaves the run as what is
// selected, and the jobs without the focus look.
func TestAnnotationsFocusActsOnTheRun(t *testing.T) {
	t.Parallel()
	m, h := newModal(t, newFake(), wideW, wideH)
	h.toJobs(t)
	if sel, _ := m.Selected(); sel.What != "job" {
		t.Fatalf("with the jobs focused, Selected is %q, want job", sel.What)
	}
	focused := m.View()
	h.keys("2")
	if sel, _ := m.Selected(); sel.What != "run" {
		t.Errorf("with the annotations focused, Selected is %q, want run", sel.What)
	}
	if m.inRuns() {
		t.Errorf("with the annotations focused, the runs have the focus")
	}
	if m.View() == focused {
		t.Errorf("the jobs look the same with the annotations focused")
	}
}

func TestTinySizes(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{1, 1}, {20, 2}, {100, 3}, {100, 4}} {
		for _, keys := range [][]string{nil, {"z"}, {"2"}, {"z", "2"}, {"enter", "j", "2"}} {
			m, h := newModal(t, newFake(), size[0], size[1])
			h.keys(keys...)
			assertFits(t, m.View(), size[0], size[1])
		}
	}
}

// Zoom does nothing below the wide layout, and so the help doesn't offer it;
// Back from the annotations names the pane the focus returns to.
func TestHelpOfPanes(t *testing.T) {
	t.Parallel()
	m, _ := newModal(t, newFake(), 80, 24)
	if k := m.keys.state(m); k.Zoom.Enabled() {
		t.Errorf("Zoom is offered at 80 columns")
	}
	m, _ = newModal(t, newFake(), 120, 30)
	if k := m.keys.state(m); !k.Zoom.Enabled() {
		t.Errorf("Zoom is not offered at 120 columns")
	}

	m, h := newModal(t, newFake(), 120, 30)
	h.keys("2")
	if k := m.keys.state(m); !k.Back.Enabled() || k.Back.Help().Desc != "runs" {
		t.Errorf("Back from the annotations over the runs is %v %q, want runs", k.Back.Enabled(), k.Back.Help().Desc)
	}
}
