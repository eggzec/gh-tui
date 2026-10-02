package actions

import (
	"slices"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// took returns the runs whose jobs and the jobs whose logs f read since the
// last call, in the order of their IDs, since reads ahead run at once, and
// forgets them.
func (f *fake) took() (jobs, logs []int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	jobs, logs = f.jobReads, f.logReads
	f.jobReads, f.logReads = nil, nil
	slices.Sort(jobs)
	slices.Sort(logs)
	return jobs, logs
}

func TestJobsAroundTheRunsAreReadAhead(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH, withPrefetch(nil))
	// The run under the cursor, then the two below it; none is above.
	if got, _ := f.took(); !slices.Equal(got, []int64{runningRun, passedRun, failedRun}) {
		t.Errorf("read the jobs of %v, want those of the first run and the two below it", got)
	}
	// The next run's jobs show before the cursor rests, and the rest reads
	// the run the window reaches now, as well as the run itself again.
	h.hold = func(msg tea.Msg) bool { _, ok := msg.(restMsg); return ok }
	h.keys("j")
	if !m.jobs.loaded || m.jobs.runID != runningRun {
		t.Fatalf("jobs pane shows run %d, loaded %v; want the next run's jobs at once", m.jobs.runID, m.jobs.loaded)
	}
	if got, _ := f.took(); !slices.Equal(got, []int64{cancelledRun}) {
		t.Errorf("read the jobs of %v, want only those of the run the window reaches", got)
	}
	h.hold = nil
	h.release()
	if got, _ := f.took(); !slices.Equal(got, []int64{runningRun}) {
		t.Errorf("after the rest, read the jobs of %v, want those of the run under the cursor", got)
	}
}

func TestJobsAheadAreConfigured(t *testing.T) {
	for name, edit := range map[string]func(p *config.PrefetchLayers){
		"tests' options": nil,
		"no window":      func(p *config.PrefetchLayers) { p.Actions.Window = config.Span{Before: new(0), After: new(0)} },
		"off":            func(p *config.PrefetchLayers) { p.Actions.Jobs.Enabled = new(false) },
		"all off":        func(p *config.PrefetchLayers) { p.Enabled = false },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			opts := []Option{withPrefetch(edit)}
			if edit == nil {
				// forTests reads nothing ahead.
				opts = nil
			}
			_, _ = newModal(t, f, wideW, wideH, opts...)
			if got, _ := f.took(); !slices.Equal(got, []int64{failedRun}) {
				t.Errorf("read the jobs of %v, want only those of the run under the cursor", got)
			}
		})
	}
}

// failTwice makes the macOS job of the failed run fail too, after the
// Ubuntu one.
func failTwice(f *fake) {
	jobs := f.jobs[failedRun]
	for i := range jobs {
		if jobs[i].ID == macosJob {
			jobs[i].Conclusion = core.ConclusionFailure
		}
	}
}

func TestLogsOfFailedJobsAreReadAheadWhenOn(t *testing.T) {
	f := newFake()
	failTwice(f)
	m, h := newModal(t, f, wideW, wideH, withPrefetch(func(p *config.PrefetchLayers) {
		p.Actions.Jobs.Enabled = new(false)
		p.Actions.Logs.Enabled = new(true)
	}))
	// The log pane reads the Ubuntu job under the cursor, and the window
	// below it the macOS job that failed too, but not the skipped build.
	if _, got := f.took(); !slices.Equal(got, []int64{ubuntuJob, macosJob}) {
		t.Errorf("read the logs of %v, want the job under the cursor's and the failed one below it", got)
	}
	// So the macOS job's log shows at once.
	h.keys("enter", "j")
	if _, got := f.took(); len(got) != 0 {
		t.Errorf("read the logs of %v on the next job, want none", got)
	}
	if j, ok := m.log.Job(); !ok || j.ID != macosJob {
		t.Errorf("the log pane shows job %d, want the macOS job", j.ID)
	}
}

func TestLogsAreNotReadAheadByDefault(t *testing.T) {
	f := newFake()
	failTwice(f)
	_, _ = newModal(t, f, wideW, wideH, withPrefetch(nil))
	if _, got := f.took(); !slices.Equal(got, []int64{ubuntuJob}) {
		t.Errorf("read the logs of %v, want only the job under the cursor's", got)
	}
}

// detach runs cmd, and the commands of a batch it returns, apart, and
// returns the func that waits for them to end. Their messages are dropped.
func detach(cmd tea.Cmd) (wait func()) {
	var wg sync.WaitGroup
	var spawn func(tea.Cmd)
	spawn = func(c tea.Cmd) {
		if c == nil {
			return
		}
		wg.Go(func() {
			if b, ok := c().(tea.BatchMsg); ok {
				for _, sub := range b {
					spawn(sub)
				}
			}
		})
	}
	spawn(cmd)
	return wg.Wait
}

// holdLogAhead reads the log of the macOS job ahead, from the Ubuntu job
// under the cursor, and holds that read in flight. It returns the func that
// waits for the read to end.
func holdLogAhead(t *testing.T, f *fake) (m *Modal, h *host, wait func()) {
	t.Helper()
	failTwice(f)
	f.started = make(chan int64, 1)
	f.release = make(chan struct{})
	m, h = newModal(t, f, wideW, wideH, withPrefetch(func(p *config.PrefetchLayers) {
		p.Actions.Jobs.Enabled = new(false)
		p.Actions.Logs.Enabled = new(true)
	}))
	// Leave the window empty, and have the macOS job's log to read again
	// once the cursor is back.
	h.keys("enter", "j")
	f.mu.Lock()
	f.cachedLogs[macosJob] = false
	f.logReads = nil
	f.holdLogs = map[int64]bool{macosJob: true}
	f.mu.Unlock()
	h.hold = func(msg tea.Msg) bool { _, ok := msg.(ui.AheadMsg); return ok }
	h.keys("k")
	h.hold = nil
	if len(h.held) != 1 {
		t.Fatalf("the cursor moved and %d rests are due, want 1", len(h.held))
	}
	rest := h.held[0]
	h.held = nil
	wait = detach(m.Update(rest))
	if got := <-f.started; got != macosJob {
		t.Fatalf("read the log of job %d ahead, want the macOS job's", got)
	}
	return m, h, wait
}

// checkLogCancelled ends the read held by holdLogAhead, which the change
// of the list must have cancelled already, and checks that nothing more is
// read of the old run's logs.
func checkLogCancelled(t *testing.T, f *fake, wait func()) {
	t.Helper()
	close(f.release)
	wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Equal(f.canceled, []int64{macosJob}) {
		t.Errorf("the reads of the logs of %v were cancelled, want the macOS job's", f.canceled)
	}
	if n := countOf(f.logReads, macosJob) + countOf(f.logReads, ubuntuJob); n != 1 {
		t.Errorf("read the logs of the old run's jobs %v, want only the held read", f.logReads)
	}
}

func countOf(xs []int64, x int64) int {
	n := 0
	for _, v := range xs {
		if v == x {
			n++
		}
	}
	return n
}

func TestLogsAheadStopWhenTheRunChanges(t *testing.T) {
	f := newFake()
	m, h, wait := holdLogAhead(t, f)
	m.focusPane(runsPane)
	h.keys("j")
	if m.run.ID == failedRun {
		t.Fatal("the run didn't change")
	}
	checkLogCancelled(t, f, wait)
}

func TestLogsAheadStopWhenTheFilterChanges(t *testing.T) {
	f := newFake()
	m, h, wait := holdLogAhead(t, f)
	h.run(m.setFilter(core.RunFilter{Status: "success"}))
	checkLogCancelled(t, f, wait)
}
