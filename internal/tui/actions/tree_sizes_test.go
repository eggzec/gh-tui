package actions

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// passingJobs gives the failed run n jobs that all passed.
func passingJobs(f *fake, n int) {
	jobs := make([]core.Job, 0, n)
	for i := range n {
		jobs = append(jobs, job(int64(1000+i), failedRun, fmt.Sprintf("job-%02d", i), core.ConclusionSuccess, 0, 0))
	}
	f.jobs[failedRun] = jobs
}

// With more jobs than rows the cursor row always shows, at 80 and 120
// columns, and the run folds and unfolds with enter.
func TestTreeKeepsTheCursorInView(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{narrowW, narrowH}, {100, 30}} {
		f := newFake()
		passingJobs(f, 30)
		m, h := newModal(t, f, size[0], size[1])
		for i := range 30 {
			h.keys("j")
			if want := fmt.Sprintf("job-%02d", i); !strings.Contains(screen(m), want) {
				t.Fatalf("%dx%d: %s is out of view with the cursor on it:\n%s", size[0], size[1], want, screen(m))
			}
			assertFits(t, m.View(), size[0], size[1])
		}
		h.keys("backspace")
		h.keys("enter")
		if s := screen(m); strings.Contains(s, "job-00") {
			t.Errorf("%dx%d: the folded run still shows its jobs:\n%s", size[0], size[1], s)
		}
		h.keys("enter")
		if s := screen(m); !strings.Contains(s, "job-") {
			t.Errorf("%dx%d: the unfolded run shows no jobs:\n%s", size[0], size[1], s)
		}
	}
}

// A prompt or a chip under the runs takes a row of the feed, and the selected
// run still shows over its jobs.
func TestTreeKeepsTheRunWithAFooter(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		for _, keys := range [][]string{{"/"}, {"/", "l", "enter"}} {
			f := newFake()
			passingJobs(f, 30)
			m, h := newModal(t, f, size[0], size[1])
			h.keys(keys...)
			if row := m.runs.SelectedRow(); row < 0 || !strings.Contains(screen(m), "#4812") {
				t.Errorf("%dx%d after %v: the selected run is off screen:\n%s", size[0], size[1], keys, screen(m))
			}
			assertFits(t, m.View(), size[0], size[1])
		}
	}
}

// The tree draws in any size, however small.
func TestTreeFitsTinySizes(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{1, 1}, {20, 2}, {100, 3}} {
		for _, keys := range [][]string{nil, {"j"}, {"enter"}, {"j", "j", "enter"}} {
			f := newFake()
			passingJobs(f, 30)
			m, h := newModal(t, f, wideW, wideH)
			h.keys(keys...)
			m.SetSize(size[0], size[1])
			assertFits(t, m.View(), size[0], size[1])
		}
	}
}
