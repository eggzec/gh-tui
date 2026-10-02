package actions

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// TestOnlineReadsKeptAgain checks that the runs, the jobs of the run
// shown and the annotations of the job shown, served kept while GitHub
// rate limited their reads, are read again once the limit lifts, once.
func TestOnlineReadsKeptAgain(t *testing.T) {
	f := newFake()
	f.limited = true
	_, h := newModal(t, f, 120, 30)
	h.keys("enter")
	f.mu.Lock()
	f.limited = false
	runs, jobs, notes := len(f.queries), len(f.jobReads), len(f.noteReads)
	f.mu.Unlock()

	h.send(ui.OnlineMsg{})
	h.send(ui.OnlineMsg{})
	f.mu.Lock()
	defer f.mu.Unlock()
	if got := len(f.queries) - runs; got != 1 {
		t.Errorf("runs read after two OnlineMsg = %d, want 1", got)
	}
	if got := f.jobReads[jobs:]; len(got) != 1 || got[0] != failedRun {
		t.Errorf("jobs read after two OnlineMsg = %v, want those of run %d once", got, failedRun)
	}
	if got := f.noteReads[notes:]; len(got) != 1 || got[0] != ubuntuJob {
		t.Errorf("annotations read after two OnlineMsg = %v, want those of job %d once", got, ubuntuJob)
	}
}

// TestOnlineReadsKeptWorkflowsAgain checks that the workflows of the
// filter, served kept while GitHub rate limited their read, are read
// again once the limit lifts, once.
func TestOnlineReadsKeptWorkflowsAgain(t *testing.T) {
	f := newFake()
	f.limited = true
	m, h := newModal(t, f, wideW, wideH)
	h.keys("f")
	if !m.workflows.loaded {
		t.Fatal("the filter didn't read the workflows")
	}
	f.mu.Lock()
	f.limited = false
	reads := f.wfReads
	f.mu.Unlock()

	h.send(ui.OnlineMsg{})
	h.send(ui.OnlineMsg{})
	f.mu.Lock()
	defer f.mu.Unlock()
	if got := f.wfReads - reads; got != 1 {
		t.Errorf("workflows read after two OnlineMsg = %d, want 1", got)
	}
}

// TestPollDuringJobsReadKeepsItLoading checks that a poll of the run
// shown, which shows its cached jobs while a read of them is in flight,
// doesn't let the next wake start a second read.
func TestPollDuringJobsReadKeepsItLoading(t *testing.T) {
	f := newFake()
	fl := &follows{}
	m, h := newModal(t, f, wideW, wideH, WithFollow(fl.follow))
	h.keys("j")
	if !m.hasRun || m.run.ID != runningRun || !m.jobs.loaded {
		t.Fatalf("the running run's jobs didn't show")
	}
	// A read of the jobs failed for the rate limit; the jobs shown stay.
	m.jobs.err = fmt.Errorf("jobs: %w", core.ErrRateLimited)
	h.hold = func(msg tea.Msg) bool { _, ok := msg.(jobsMsg); return ok }
	f.mu.Lock()
	reads := len(f.jobReads)
	f.mu.Unlock()

	h.send(ui.OnlineMsg{})
	h.send(ui.SyncMsg{Key: actionssvc.RunSyncKey(repo, runningRun)})
	h.send(ui.OnlineMsg{})
	f.mu.Lock()
	got := f.jobReads[reads:]
	f.mu.Unlock()
	if len(got) != 1 || got[0] != runningRun {
		t.Errorf("jobs read = %v, want those of run %d once", got, runningRun)
	}
	h.hold = nil
	h.release()
	if m.jobs.loading || m.jobs.err != nil {
		t.Errorf("the read's answer left loading %v, err %v", m.jobs.loading, m.jobs.err)
	}
}

// TestReopenReadsWhatWasLost checks that the jobs, the workflows and the
// annotations whose reads were in flight while a preview hid the modal,
// whose answers went to the preview, are read again once it reopens, so
// that nothing stays loading, and that the next wake reads what is kept.
func TestReopenReadsWhatWasLost(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("enter")
	// Reads start, and a preview hides the modal before they answer.
	h.hold = func(tea.Msg) bool { return true }
	h.run(m.rereadJobs())
	h.run(m.openFilter())
	f.mu.Lock()
	f.cachedNotes[ubuntuJob] = false
	f.mu.Unlock()
	m.log.Clear()
	h.run(m.showJob(false))
	h.held, h.hold = nil, nil
	if !m.jobs.loading || !m.workflows.loading {
		t.Fatalf("loading jobs %v, workflows %v; want both", m.jobs.loading, m.workflows.loading)
	}
	f.mu.Lock()
	f.limited = true
	jobs, wfs, notes := len(f.jobReads), f.wfReads, len(f.noteReads)
	f.mu.Unlock()

	h.send(ui.ReopenedMsg{Modal: m})
	f.mu.Lock()
	gotJobs, gotWfs, gotNotes := len(f.jobReads)-jobs, f.wfReads-wfs, len(f.noteReads)-notes
	f.limited = false
	f.mu.Unlock()
	if gotJobs != 1 || gotWfs != 1 || gotNotes != 1 {
		t.Errorf("read on reopen: %d jobs, %d workflows, %d annotations; want each once", gotJobs, gotWfs, gotNotes)
	}
	if m.loading() {
		t.Errorf("still loading once reopened: jobs %v, workflows %v", m.jobs.loading, m.workflows.loading)
	}
	// What the reopen read was served kept, so a wake reads it again.
	h.send(ui.OnlineMsg{})
	f.mu.Lock()
	defer f.mu.Unlock()
	if n := len(f.jobReads) - jobs - gotJobs; n != 1 {
		t.Errorf("jobs read on the wake %d times, want 1", n)
	}
}

// TestWorkflowsReadAgainAfterAFailedWake checks that workflows served
// kept are marked again when their read on a wake gets no answer, so that
// the next wake tries again.
func TestWorkflowsReadAgainAfterAFailedWake(t *testing.T) {
	f := newFake()
	f.limited = true
	m, h := newModal(t, f, wideW, wideH)
	h.keys("f")
	f.mu.Lock()
	f.limited = false
	f.wfErr = fmt.Errorf("list workflows: %w", core.ErrOffline)
	f.mu.Unlock()
	h.send(ui.OnlineMsg{})
	if !m.workflows.kept {
		t.Fatal("the workflows aren't marked to be read again")
	}
	f.mu.Lock()
	f.wfErr = nil
	reads := f.wfReads
	f.mu.Unlock()
	h.send(ui.OnlineMsg{})
	f.mu.Lock()
	defer f.mu.Unlock()
	if n := f.wfReads - reads; n != 1 {
		t.Errorf("workflows read on the next wake %d times, want 1", n)
	}
}
