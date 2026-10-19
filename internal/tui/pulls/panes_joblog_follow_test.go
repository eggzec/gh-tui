package pulls

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// runFollows counts the follows of runs that were started and ended.
type runFollows struct {
	runs          []int64
	starts, stops atomic.Int32
}

func (f *runFollows) follow(_ core.RepoRef, run int64) func() {
	f.runs = append(f.runs, run)
	f.starts.Add(1)
	return func() { f.stops.Add(1) }
}

// runningJob is a check that is a job in progress.
func runningJob() core.CommitCheck {
	return core.CommitCheck{ID: 5, JobID: 5, RunID: 9, Name: "build", Status: core.RunInProgress}
}

func textLines(texts ...string) []core.LogLine {
	out := make([]core.LogLine, len(texts))
	for i, s := range texts {
		out[i] = core.LogLine{Text: s, Step: 1}
	}
	return out
}

func TestRunningJobLogFollowsItsRun(t *testing.T) {
	t.Parallel()
	svc := &logChecks{fakeChecks: &fakeChecks{}}
	svc.partial = core.PartialLog{Lines: textLines("compiling", "testing"), Gen: 1}
	m, svc := jobPanesOf(t, runningJob(), svc)
	fl := &runFollows{}
	m.merge.follow = fl.follow
	hoverJob(t, m)

	// hoverJob leaves the row and comes back, so what runs now is what started
	// and didn't end.
	live := func() (watches, follows int32) {
		return svc.watches.Load() - svc.stops.Load(), fl.starts.Load() - fl.stops.Load()
	}
	if w, f := live(); w != 1 || f != 1 || fl.runs[0] != 9 {
		t.Fatalf("%d watches and %d follows run, of runs %v, want one each of the run of the job", w, f, fl.runs)
	}
	if body := ansi.Strip(m.View()); !strings.Contains(body, "testing") {
		t.Fatalf("the preview lacks the log read:\n%s", body)
	}

	// The poll of the run adds lines, and tells the app with a sync of its key.
	run := ui.SyncMsg{Key: actionssvc.RunSyncKey(m.repo, 9)}
	svc.partial.Lines = append(svc.partial.Lines[:2:2], core.LogLine{Text: "fresh line", Step: 1})
	svc.partial.At = time.Unix(100, 0)
	m.Update(run)
	if body := ansi.Strip(m.View()); !strings.Contains(body, "fresh line") {
		t.Errorf("the preview lacks the lines the poll added:\n%s", body)
	}

	// A log that started over with as many lines is shown too.
	svc.partial = core.PartialLog{Lines: textLines("again", "from", "scratch"), Gen: 2, At: time.Unix(200, 0)}
	m.Update(run)
	if body := ansi.Strip(m.View()); !strings.Contains(body, "scratch") || strings.Contains(body, "fresh line") {
		t.Errorf("the preview kept the lines of the old log:\n%s", body)
	}

	// Leaving the row ends the watch and the follow, and coming back starts them.
	feedLog(m, m.Update(keyMsg("k")))
	if w, f := live(); w != 0 || f != 0 {
		t.Errorf("after leaving, %d watches and %d follows still run", w, f)
	}
	feedLog(m, m.Update(keyMsg("j")))
	if w, f := live(); w != 1 || f != 1 {
		t.Errorf("after coming back, %d watches and %d follows run, want one each", w, f)
	}

	// Closing the modal ends them too.
	m.Discard()
	if w, f := live(); w != 0 || f != 0 {
		t.Errorf("after closing, %d watches and %d follows still run", w, f)
	}
}

func TestCompletedJobIsNotFollowed(t *testing.T) {
	t.Parallel()
	m, svc := jobPanes(t)
	fl := &runFollows{}
	m.merge.follow = fl.follow
	hoverJob(t, m)
	if fl.starts.Load() != 0 || svc.watches.Load() != 0 {
		t.Errorf("a finished job was followed: %d follows, %d watches", fl.starts.Load(), svc.watches.Load())
	}
}

func TestFirstErrorUsesStepsReadWithTheLog(t *testing.T) {
	t.Parallel()
	// A passing step prints an error line before the step that failed.
	lines := []core.LogLine{
		{Text: "setup", Step: 1},
		{Text: "##[error]ignored, the step goes on", Step: 1, Kind: core.LogError},
		{Text: "build", Step: 2},
		{Text: "test", Step: 2},
		{Text: "boom", Step: 2, Kind: core.LogError},
	}
	svc := &logChecks{fakeChecks: &fakeChecks{}, lines: lines}
	svc.jobs = core.Page[core.Job]{Items: []core.Job{{ID: 5, RunID: 9, Steps: []core.Step{
		{Number: 1, Conclusion: core.ConclusionSuccess},
		{Number: 2, Conclusion: core.ConclusionFailure},
	}}}}
	m, svc := jobPanesOf(t, completedJob(), svc)
	hoverJob(t, m)
	l := m.merge.logs[5]
	if l == nil || !l.found || l.first != 4 {
		t.Fatalf("the first error is %+v, want the line of the failed step", l)
	}
	if got := svc.jobReads.Load(); got != 1 {
		t.Errorf("the jobs were read %d times, want once", got)
	}
}

func TestPendingLogOfFinishedJobIsReadOnce(t *testing.T) {
	t.Parallel()
	svc := &logChecks{fakeChecks: &fakeChecks{}, logErr: core.ErrLogPending}
	m, svc := jobPanesOf(t, completedJob(), svc)
	hoverJob(t, m)
	var cmd tea.Cmd
	for range 8 {
		if msg, ok := find[mergeLogMsg](cmd); ok {
			cmd = m.Update(msg)
			continue
		}
		cmd = m.Update(ui.SyncMsg{Key: "other"})
	}
	if got := svc.reads.Load(); got != 1 {
		t.Errorf("the pending log was read %d times, want once", got)
	}
	if body := ansi.Strip(m.View()); !strings.Contains(body, "isn't published yet") {
		t.Errorf("the preview doesn't say the log is pending:\n%s", body)
	}
}

func TestLongLogJumpsToItsFirstError(t *testing.T) {
	t.Parallel()
	const total, errAt = 3000, 1500
	lines := make([]core.LogLine, total)
	for i := range lines {
		lines[i] = core.LogLine{Text: fmt.Sprintf("line %d", i), Step: 1}
	}
	lines[errAt].Kind = core.LogError
	m, _ := jobPanesOf(t, completedJob(), &logChecks{fakeChecks: &fakeChecks{}, lines: lines})
	hoverJob(t, m)

	body, ok := m.jobLogBody(jobRow(5), 100)
	if !ok {
		t.Fatal("the row of the job shows no log")
	}
	rows := strings.Split(ansi.Strip(body), "\n")
	if got, want := len(rows), shownLines+2; got != want {
		t.Fatalf("the preview has %d rows, want the %d lines of the window and two notes", got, want)
	}
	if want := "500 earlier lines not shown"; rows[0] != want {
		t.Errorf("first row %q, want %q", rows[0], want)
	}
	if want := "500 more lines not shown"; rows[len(rows)-1] != want {
		t.Errorf("last row %q, want %q", rows[len(rows)-1], want)
	}
	off := m.previews[previewKey{panes.Merge, jobRow(5)}]
	if got := strings.TrimSpace(strings.TrimLeft(rows[off+errSeen], " ")); !strings.HasSuffix(got, fmt.Sprintf("line %d", errAt)) {
		t.Errorf("the row %d rows below the top is %q, want the first error", errSeen, got)
	}
}

func TestScrollingWhileLoadingKeepsTheJump(t *testing.T) {
	t.Parallel()
	m, _ := jobPanes(t)
	var read mergeLogMsg
	for range 12 {
		if m.hover[2].id == jobRow(5) {
			break
		}
		if msg, ok := find[mergeLogMsg](m.Update(keyMsg("j"))); ok {
			read = msg
		}
	}
	if read.job != 5 {
		t.Fatal("hovering the job started no read of its log")
	}
	// Scrolling the note that the log is loading writes a zero offset.
	key := previewKey{panes.Merge, jobRow(5)}
	m.previews[key] = 0
	m.Update(read)
	if got := m.previews[key]; got == 0 {
		t.Error("the log shows from its top, want it scrolled to the first error")
	}
}

// completedJob is a check that is a job that failed.
func completedJob() core.CommitCheck {
	return core.CommitCheck{ID: 5, JobID: 5, RunID: 9, Name: "build", Status: core.RunCompleted, Conclusion: core.ConclusionFailure}
}
