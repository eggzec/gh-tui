package actions

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

const commitSHA = "abc1234"

// onCommit gives the runs of ids the commit sha.
func onCommit(f *fake, sha string, ids ...int64) {
	for i := range f.runs {
		for _, id := range ids {
			if f.runs[i].ID == id {
				f.runs[i].HeadSHA = sha
			}
		}
	}
}

func TestOpensOnTheRunsOfACommit(t *testing.T) {
	t.Parallel()
	f := newFake()
	onCommit(f, commitSHA, passedRun, cancelledRun)
	m, _ := newModal(t, f, wideW, wideH, WithFilter(core.RunFilter{HeadSHA: commitSHA}))
	if q := f.queries[0]; q.Filter.HeadSHA != commitSHA {
		t.Errorf("first query %+v, want the runs of commit %s", q, commitSHA)
	}
	if m.runs.Len() != 2 || m.run.ID != passedRun {
		t.Errorf("shows %d runs on run %d, want the 2 of the commit on %d", m.runs.Len(), m.run.ID, passedRun)
	}
	// The commit stays when the filter form is cleared.
	if got := cleared(core.RunFilter{Branch: "main", HeadSHA: commitSHA}); got.HeadSHA != commitSHA {
		t.Errorf("clearing the form leaves %+v, want the commit kept", got)
	}
}

func TestOpensOnARun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		page    int
		run     int64
		want    int64
		queries int
	}{
		{"on the first page", 0, passedRun, passedRun, 1},
		{"on the second page", 2, passedRun, passedRun, 2},
		{"on the last page", 1, cancelledRun, cancelledRun, 4},
		{"missing from one page", 0, 999, failedRun, 1},
		{"missing from every page", 2, 999, failedRun, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFake()
			f.runPage = tt.page
			m, _ := newModal(t, f, wideW, wideH, WithRun(tt.run))
			if !m.hasRun || m.run.ID != tt.want {
				t.Errorf("shows run %d, want %d", m.run.ID, tt.want)
			}
			if r, ok := m.runs.Selected(); !ok || r.ID != tt.want {
				t.Errorf("the runs select %d, want %d", r.ID, tt.want)
			}
			if len(f.queries) != tt.queries {
				t.Errorf("%d queries of the runs, want %d: %+v", len(f.queries), tt.queries, f.queries)
			}
			if m.runs.Seeking() || m.runs.Err() != nil {
				t.Errorf("seeking %v with error %v, want it settled without one", m.runs.Seeking(), m.runs.Err())
			}
			if m.focus != runsPane {
				t.Errorf("focus is on pane %d, want the runs", m.focus)
			}
			// Only the run opened on has its jobs read, not the first one on
			// the way.
			if jobs := f.jobReads; len(jobs) != 1 || jobs[0] != tt.want {
				t.Errorf("read the jobs of runs %v, want only of %d", jobs, tt.want)
			}
		})
	}
}

func TestOpensOnARunOfACommit(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runPage = 1
	onCommit(f, commitSHA, passedRun, cancelledRun)
	m, _ := newModal(t, f, wideW, wideH, WithFilter(core.RunFilter{HeadSHA: commitSHA}), WithRun(cancelledRun))
	if m.run.ID != cancelledRun {
		t.Errorf("shows run %d, want %d", m.run.ID, cancelledRun)
	}
	if len(f.queries) != 2 {
		t.Errorf("%d queries, want 2 pages of the 2 runs of the commit", len(f.queries))
	}
	for _, q := range f.queries {
		if q.Filter.HeadSHA != commitSHA {
			t.Errorf("query %+v lacks the commit", q)
		}
	}
}

func TestOpensOnAJob(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts []Option
		want int64
	}{
		{"a passing one", []Option{WithJob(macosJob)}, macosJob},
		{"the failed one", []Option{WithJob(ubuntuJob)}, ubuntuJob},
		{"one the run lacks", []Option{WithJob(7)}, ubuntuJob},
		{"none asked", nil, ubuntuJob},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFake()
			m, _ := newModal(t, f, wideW, wideH, tt.opts...)
			if m.run.ID != failedRun {
				t.Fatalf("shows run %d, want %d", m.run.ID, failedRun)
			}
			if j, ok := m.jobs.selected(); !ok || j.ID != tt.want {
				t.Errorf("the jobs select %d, want %d", j.ID, tt.want)
			}
			if m.wantJob != 0 {
				t.Errorf("still wants job %d once the jobs showed", m.wantJob)
			}
			if m.focus != runsPane {
				t.Errorf("focus is on pane %d, want the runs", m.focus)
			}
		})
	}
}

func TestOpensOnAJobOfARun(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runPage = 2
	f.logs[300] = f.logs[ubuntuJob]
	m, _ := newModal(t, f, wideW, wideH, WithRun(passedRun), WithJob(300), WithJobsFocused())
	if m.run.ID != passedRun {
		t.Fatalf("shows run %d, want %d", m.run.ID, passedRun)
	}
	if j, _ := m.jobs.selected(); j.ID != 300 {
		t.Errorf("the jobs select %d, want 300", j.ID)
	}
	if m.focus != jobsPane {
		t.Errorf("focus is on pane %d, want the jobs", m.focus)
	}
}

func TestOpensWithTheJobsFocused(t *testing.T) {
	t.Parallel()
	f := newFake()
	m, _ := newModal(t, f, wideW, wideH, WithJobsFocused())
	if m.focus != jobsPane {
		t.Errorf("focus is on pane %d, want the jobs", m.focus)
	}
	if j, _ := m.jobs.selected(); j.ID != ubuntuJob {
		t.Errorf("the jobs select %d, want the failed one, %d", j.ID, ubuntuJob)
	}
}

func TestOpensOnAFailedJobAtItsError(t *testing.T) {
	t.Parallel()
	f := newFake()
	m, h := newModal(t, f, wideW, wideH, WithRun(failedRun), WithJob(ubuntuJob), WithJobsFocused())
	if m.focus != jobsPane {
		t.Errorf("focus is on pane %d, want the jobs", m.focus)
	}
	h.keys("enter")
	if h.log == nil || h.log.log.State() != jobview.Ready || h.log.log.Errors() != 1 {
		t.Fatal("enter didn't open the log of the failed job with its error")
	}
	if runs, jobs, logs := f.counts(); runs != 1 || jobs != 1 || logs != 1 {
		t.Errorf("opening read %d pages of runs, %d of jobs and %d logs, want one each", runs, jobs, logs)
	}
}

func TestOpenerOpensWhereTheAppSays(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runPage = 2
	onCommit(f, commitSHA, passedRun, cancelledRun)
	mod, load := Opener(f, testKeys(), forTests(), withIcons(), WithViewer(viewer))(t.Context(), repo,
		core.RunFilter{HeadSHA: commitSHA}, ui.ActionsAt{Run: cancelledRun, Job: 400, Jobs: true})
	m := mod.(*Modal)
	m.SetTheme(testTheme())
	m.SetSize(wideW, wideH)
	(&host{m: m}).run(load)
	if m.run.ID != cancelledRun || m.focus != jobsPane {
		t.Errorf("opened on run %d with pane %d focused, want run %d and the jobs", m.run.ID, m.focus, cancelledRun)
	}
	if j, _ := m.jobs.selected(); j.ID != 400 {
		t.Errorf("the jobs select %d, want 400", j.ID)
	}
	if q := f.queries[0]; q.Filter.HeadSHA != commitSHA {
		t.Errorf("first query %+v, want the runs of the commit", q)
	}

	// Without a target it opens as before.
	mod, load = Opener(f, testKeys(), forTests(), withIcons())(t.Context(), repo, core.RunFilter{}, ui.ActionsAt{})
	m = mod.(*Modal)
	m.SetTheme(testTheme())
	m.SetSize(wideW, wideH)
	(&host{m: m}).run(load)
	if m.focus != runsPane || m.runs.Seeking() {
		t.Errorf("without a target the pane %d is focused, seeking %v, want the runs and no seek", m.focus, m.runs.Seeking())
	}
}

// A key of the user's ends the search for the run, however many pages are
// left: the run under the cursor shows its jobs at once, and the page that
// arrives later leaves the cursor where it is.
func TestOpensOnARunUntilTheUserMoves(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.runPage = 2
	m := New(t.Context(), f, repo, testKeys(), forTests(), withIcons(), WithViewer(viewer), WithRun(cancelledRun))
	// The runs are flat, so the run after the cancelled one is the running one.
	ungroup(m)
	m.SetTheme(testTheme())
	m.SetSize(wideW, wideH)
	h := &host{m: m}
	pages := 0
	h.hold = func(msg tea.Msg) bool {
		if !strings.HasSuffix(fmt.Sprintf("%T", msg), "chunkMsg[github.com/eggzec/gh-tui/internal/core.Run]") {
			return false
		}
		pages++
		return pages > 1
	}
	h.run(m.Init())
	if !m.runs.Seeking() || len(h.held) != 1 {
		t.Fatalf("seeking %v with %d pages held, want the search for the run to wait on the second page", m.runs.Seeking(), len(h.held))
	}

	h.keys("j")
	if m.runs.Seeking() {
		t.Error("still seeking after a key")
	}
	if m.run.ID != runningRun {
		t.Errorf("shows run %d after j, want %d", m.run.ID, runningRun)
	}
	if jobs := f.jobReads; len(jobs) != 1 || jobs[0] != runningRun {
		t.Errorf("read the jobs of runs %v, want only of %d", jobs, runningRun)
	}

	h.hold = nil
	h.release()
	if r, _ := m.runs.Selected(); r.ID != runningRun || m.run.ID != runningRun {
		t.Errorf("the runs select %d and show %d once the page came, want %d", r.ID, m.run.ID, runningRun)
	}
	if m.runs.Seeking() || m.runs.Len() != 4 {
		t.Errorf("seeking %v with %d runs, want the page kept and no search", m.runs.Seeking(), m.runs.Len())
	}
}

// With the jobs focused on a job the opener chose, the modal looks the same
// wide as it does as one pane.
func TestOpenerView(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		width, height int
	}{
		{"190 columns", wideW, wideH},
		{"80 columns", narrowW, narrowH},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, _ := newModal(t, newFake(), tt.width, tt.height, WithRun(failedRun), WithJob(ubuntuJob), WithJobsFocused())
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

func TestOpensAsBeforeWithoutOptions(t *testing.T) {
	t.Parallel()
	f := newFake()
	m, _ := newModal(t, f, wideW, wideH)
	if m.focus != runsPane {
		t.Errorf("focus is on pane %d, want the runs", m.focus)
	}
	if m.runs.Seeking() {
		t.Error("seeks a run nobody asked for")
	}
	if m.run.ID != failedRun || len(f.queries) != 1 {
		t.Errorf("shows run %d after %d queries, want the newest after one", m.run.ID, len(f.queries))
	}
	if runs, jobs, logs := f.counts(); runs != 1 || jobs != 1 || logs != 0 {
		t.Errorf("opening read %d pages of runs, %d of jobs and %d logs, want one of each page and no log", runs, jobs, logs)
	}
}
