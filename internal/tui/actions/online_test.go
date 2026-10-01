package actions

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// TestOnlineReadsKeptAgain checks that the runs, and the jobs of the run
// shown, served kept while GitHub rate limited their reads, are read
// again once the limit lifts, once.
func TestOnlineReadsKeptAgain(t *testing.T) {
	f := newFake()
	f.limited = true
	_, h := newModal(t, f, 120, 30)
	h.keys("enter")
	f.mu.Lock()
	f.limited = false
	runs, jobs := len(f.queries), len(f.jobReads)
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
}
