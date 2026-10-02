package actions

import (
	"fmt"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
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
