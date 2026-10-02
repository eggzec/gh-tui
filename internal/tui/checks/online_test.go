package checks

import (
	"fmt"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// reads counts the reads the fake served of the checks, the jobs and the
// logs.
func (f *fake) reads() (checks, jobs, logs int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checkReads, f.jobReads, f.logReads
}

// TestOnlineRetriesOnce checks that GitHub answering again reads once
// what failed for want of an answer, the checks, the job or its log, and
// that what GitHub refused, or what loaded, costs nothing.
func TestOnlineRetriesOnce(t *testing.T) {
	offline := fmt.Errorf("github: GET: %w", core.ErrOffline)
	refused := fmt.Errorf("github: 403 Forbidden: %w", core.ErrForbidden)
	tests := []struct {
		name  string
		setup func(f *fake)
		// job opens the job under the cursor before GitHub answers.
		job                bool
		checks, jobs, logs int
	}{
		{name: "checks offline", setup: func(f *fake) { f.checksErr = offline }, checks: 1},
		{name: "checks refused", setup: func(f *fake) { f.checksErr = refused }},
		{name: "job offline", setup: func(f *fake) { f.jobsErr = offline }, job: true, jobs: 1, logs: 1},
		{name: "job refused", setup: func(f *fake) { f.jobsErr = refused }, job: true},
		{name: "log offline", setup: func(f *fake) { f.logErr = offline }, job: true, logs: 1},
		{name: "loaded", setup: func(*fake) {}, job: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			tt.setup(f)
			s, h := newStep(t, f, wideW, wideH)
			if tt.job {
				h.keys("enter")
			}
			f.mu.Lock()
			f.checksErr, f.jobsErr, f.logErr = nil, nil, nil
			f.mu.Unlock()
			checks, jobs, logs := f.reads()

			h.send(ui.OnlineMsg{})
			h.send(ui.OnlineMsg{})
			c, j, l := f.reads()
			if c-checks != tt.checks || j-jobs != tt.jobs || l-logs != tt.logs {
				t.Errorf("reads after two OnlineMsg: %d of the checks, %d of the jobs, %d of the log; want %d, %d, %d",
					c-checks, j-jobs, l-logs, tt.checks, tt.jobs, tt.logs)
			}
			if tt.checks > 0 && (!s.loaded || s.err != nil) {
				t.Errorf("checks not shown once online: loaded %v, err %v", s.loaded, s.err)
			}
			if tt.logs > 0 && s.view.Err() != nil {
				t.Errorf("log still failed once online: %v", s.view.Err())
			}
		})
	}
}

// TestOnlineReadsKeptAnnotationsAgain checks that the annotations of the
// job shown, served kept while GitHub rate limited their read, are read
// again once the limit lifts, once.
func TestOnlineReadsKeptAnnotationsAgain(t *testing.T) {
	f := newFake()
	f.notesLimited = true
	_, h := newStep(t, f, wideW, wideH)
	h.keys("enter")
	f.mu.Lock()
	f.notesLimited = false
	notes := f.noteReads
	f.mu.Unlock()
	if notes == 0 {
		t.Fatal("the job's annotations weren't read")
	}

	h.send(ui.OnlineMsg{})
	h.send(ui.OnlineMsg{})
	f.mu.Lock()
	defer f.mu.Unlock()
	if got := f.noteReads - notes; got != 1 {
		t.Errorf("annotations read after two OnlineMsg = %d, want 1", got)
	}
}
