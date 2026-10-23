package actions

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// manyJobs are n jobs of the failed run, named job-01 and on.
func manyJobs(n int) []core.Job {
	jobs := make([]core.Job, n)
	for i := range jobs {
		jobs[i] = job(int64(7000+i), failedRun, fmt.Sprintf("job-%02d", i+1), core.ConclusionSuccess, time.Hour, time.Minute)
	}
	return jobs
}

// With more jobs than the section has rows, the cursor stays in view as it
// moves, and the line that says there are more jobs stays under them.
func TestJobsScrollInTheirSection(t *testing.T) {
	t.Parallel()
	const n = 30
	f := newFake()
	f.moreJobs = true
	f.setJobs(failedRun, manyJobs(n))
	m, h := newModal(t, f, 80, 24)
	h.keys("tab")
	more := fmt.Sprintf("First %d jobs · o shows all", n)
	for i := 1; i <= n; i++ {
		if i > 1 {
			h.keys("j")
		}
		s := screen(m)
		if want := fmt.Sprintf("job-%02d", i); !strings.Contains(s, want) {
			t.Fatalf("after %d presses of j the cursor job is out of view:\n%s", i-1, s)
		}
		if !strings.Contains(s, more) {
			t.Fatalf("after %d presses of j the line of more jobs is gone:\n%s", i-1, s)
		}
	}

	m, h = newModal(t, f, 80, 24)
	h.keys("tab", "G")
	s := screen(m)
	if want := fmt.Sprintf("job-%02d", n); !strings.Contains(s, want) {
		t.Errorf("G left the last job out of view:\n%s", s)
	}
	if !strings.Contains(s, more) {
		t.Errorf("G hid the line of more jobs:\n%s", s)
	}
	assertFits(t, m.View(), 80, 24)
}

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
	h.keys("tab")
	if sel, _ := m.Selected(); sel.What != "job" {
		t.Fatalf("with the jobs focused, Selected is %q, want job", sel.What)
	}
	focused := m.View()
	h.keys("2")
	if sel, _ := m.Selected(); sel.What != "run" {
		t.Errorf("with the annotations focused, Selected is %q, want run", sel.What)
	}
	if m.inJobs() || m.inRuns() {
		t.Errorf("with the annotations focused, the runs or the jobs have the focus")
	}
	if m.View() == focused {
		t.Errorf("the jobs look the same with the annotations focused")
	}
}

func TestTinySizes(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{1, 1}, {20, 2}, {100, 3}, {100, 4}} {
		for _, keys := range [][]string{nil, {"z"}, {"2"}, {"z", "2"}, {"tab", "2"}} {
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
	h.keys("1", "tab", "2")
	if k := m.keys.state(m); !k.Back.Enabled() || k.Back.Help().Desc != "jobs" {
		t.Errorf("Back from the annotations over the jobs is %v %q, want jobs", k.Back.Enabled(), k.Back.Help().Desc)
	}
}
