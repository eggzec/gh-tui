package actions

import (
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

var _ ui.Stepping = (*logModal)(nil)

// withAttempts gives the failed run's ubuntu job in the attempts listed,
// past the first, with logs in memory, and returns the modal and its host
// with the run at attempt 3.
func withAttempts(t *testing.T, attempts ...int) (*Modal, *host) {
	t.Helper()
	f := newFake()
	jobs := slices.Clone(f.jobs[failedRun])
	i := slices.IndexFunc(jobs, func(j core.Job) bool { return j.ID == ubuntuJob })
	f.logs[ubuntuJob], f.cachedLogs[ubuntuJob] = testLog(), true
	for _, a := range attempts {
		j := attempt(jobs[i], a)
		j.ID = 9000 + int64(a)
		jobs = append(jobs, j)
		f.logs[j.ID], f.cachedLogs[j.ID] = testLog(), true
	}
	f.setJobs(failedRun, jobs)
	for k := range f.runs {
		if f.runs[k].ID == failedRun {
			f.runs[k].Attempt = 3
		}
	}
	m, h := newModal(t, f, wideW, wideH)
	return m, h
}

func TestLogStepsThroughAttempts(t *testing.T) {
	t.Parallel()
	_, h := withAttempts(t, 2, 3)
	l := h.openLogOf(t, ubuntuName)
	// The log opens on the job of the attempt shown, the latest.
	if !strings.HasPrefix(l.Title(), "3/3 attempt 3  "+ubuntuName) {
		t.Errorf("title %q, want it to start with the attempt", l.Title())
	}
	if !slices.Contains(l.Commands(), ui.CommandNextFile) || !slices.Contains(l.Commands(), ui.CommandPrevFile) {
		t.Errorf("commands %v miss n and p", l.Commands())
	}
	l.log.SetPosition(pager.Position{Cursor: 2})
	want := l.log.Position()

	h.run(l.Step(-1))
	if l.job.ID != 9002 || !strings.HasPrefix(l.Title(), "2/3 attempt 2  ") {
		t.Fatalf("a step shows job %d titled %q, want attempt 2", l.job.ID, l.Title())
	}
	h.run(l.Step(-1))
	if l.job.ID != ubuntuJob {
		t.Fatalf("a second step shows job %d, want attempt 1", l.job.ID)
	}
	if cmd := l.Step(-1); cmd != nil || l.job.ID != ubuntuJob {
		t.Errorf("a step before the first attempt moved to job %d", l.job.ID)
	}
	h.run(l.Step(2))
	if l.job.ID != 9003 {
		t.Fatalf("two steps on show job %d, want attempt 3", l.job.ID)
	}
	if got := l.log.Position(); got != want {
		t.Errorf("attempt 3 came back at %+v, want %+v", got, want)
	}
}

func TestLogStepSkipsAnAttemptWithoutTheJob(t *testing.T) {
	t.Parallel()
	_, h := withAttempts(t, 3)
	l := h.openLogOf(t, ubuntuName)
	h.run(l.Step(-1))
	if l.job.ID != ubuntuJob || !strings.HasPrefix(l.Title(), "1/3 attempt 1  ") {
		t.Fatalf("a step shows job %d titled %q, want attempt 1", l.job.ID, l.Title())
	}
	h.run(l.Step(1))
	if l.job.ID != 9003 {
		t.Errorf("a step on shows job %d, want attempt 3", l.job.ID)
	}
}

func TestLogOfOneAttemptHasNoSteps(t *testing.T) {
	t.Parallel()
	_, h := newModal(t, newFake(), wideW, wideH)
	l := h.openLogOf(t, ubuntuName)
	if strings.Contains(l.Title(), "attempt") {
		t.Errorf("title %q names an attempt", l.Title())
	}
	if !slices.Equal(l.Commands(), []string{ui.CommandCopy}) {
		t.Errorf("commands %v, want copy only", l.Commands())
	}
	if l.Step(1) != nil || l.Step(-1) != nil {
		t.Error("the log of one attempt steps")
	}
}

func TestSyncLeavesAnOlderAttempt(t *testing.T) {
	t.Parallel()
	m, h := withAttempts(t, 2, 3)
	l := h.openLogOf(t, ubuntuName)
	h.run(l.Step(-2))
	l.log.SetPosition(pager.Position{Cursor: 2})
	want := l.log.Position()
	l.following = failedRun
	if cmd := l.Update(ui.SyncMsg{Key: actionssvc.RunSyncKey(m.repo, failedRun)}); cmd != nil {
		t.Error("a sync of the run shows the log of an older attempt again")
	}
	if l.job.ID != ubuntuJob || l.log.Position() != want {
		t.Errorf("a sync moved the older attempt to job %d at %+v", l.job.ID, l.log.Position())
	}
}
