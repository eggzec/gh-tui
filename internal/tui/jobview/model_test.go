package jobview

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestShowsAFailedJobOnItsError(t *testing.T) {
	f := newFake()
	m := newView(t, f, 80, 12)
	run(m, m.Show(failed(), false, Hints{}))
	if m.State() != Ready || m.Errors() != 1 || m.JobID() != failedJob {
		t.Fatalf("state %d with %d errors for job %d, want the log with its error", m.State(), m.Errors(), m.JobID())
	}
	if s := text(m); !strings.Contains(s, "Process completed with exit code 1.") {
		t.Errorf("the failed step isn't open on its error:\n%s", s)
	}
	// Showing it again, as a poll does, reads nothing.
	run(m, m.Show(failed(), false, Hints{}))
	if len(f.reads) != 1 {
		t.Errorf("read the log %d times, want once", len(f.reads))
	}
}

func TestStates(t *testing.T) {
	tests := []struct {
		name  string
		job   core.Job
		err   error
		state State
		text  string
	}{
		{"running", running(), nil, Pending, "The log is available when the job finishes. ✓ Set up job 2s ◐ Run golangci-lint 1m 18s"},
		{"not yet", failed(), fmt.Errorf("log: %w", core.ErrLogPending), Pending, "The job hasn't started yet."},
		{"expired", failed(), fmt.Errorf("log: %w", core.ErrLogExpired), Expired, "GitHub no longer keeps this log. ✓ Set up job 2s ✗ Run go test ./... 2m 58s"},
		{"failed", failed(), errBoom, Failed, "Couldn't load the log: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.errs[failedJob] = tt.err
			m := newView(t, f, 80, 12)
			run(m, m.Show(tt.job, false, Hints{}))
			if m.State() != tt.state {
				t.Fatalf("state %d, want %d", m.State(), tt.state)
			}
			if s := text(m); !strings.Contains(s, tt.text) {
				t.Errorf("the view lacks %q:\n%s", tt.text, s)
			}
			assertFits(t, m.View(), 80, 12)
		})
	}
}

func TestRetry(t *testing.T) {
	f := newFake()
	f.errs[failedJob] = errBoom
	m := newView(t, f, 80, 12)
	run(m, m.Show(failed(), false, Hints{}))
	if m.State() != Failed {
		t.Fatalf("state %d, want failed", m.State())
	}
	delete(f.errs, failedJob)
	run(m, m.Retry())
	if m.State() != Ready {
		t.Errorf("retry left state %d, want the log", m.State())
	}
	if m.Retry() != nil {
		t.Error("retry of a log that loaded reads it again")
	}
}

func TestRestWaitsUntilReadNow(t *testing.T) {
	f := newFake()
	m := newView(t, f, 80, 12, WithRest(time.Hour))
	cmd := m.Show(failed(), true, Hints{})
	if m.State() != Loading || len(f.reads) != 0 {
		t.Fatalf("a job shown to rest is in state %d after %d reads, want loading, unread", m.State(), len(f.reads))
	}
	_ = cmd // The rest would end in an hour.
	run(m, m.ReadNow())
	if m.State() != Ready || !slices.Equal(f.reads, []int64{failedJob}) {
		t.Errorf("read now left state %d after reads %v", m.State(), f.reads)
	}
	// The rest in flight is stale now.
	*m, cmd = m.Update(restMsg{id: m.id, seq: m.seq - 1})
	if cmd != nil {
		t.Error("a stale rest reads")
	}
}

func TestFromMemory(t *testing.T) {
	f := newFake()
	f.cached[failedJob], f.cachedNotes[failedJob] = true, true
	m := newView(t, f, 80, 12)
	if cmd := m.Show(failed(), true, Hints{}); cmd != nil || m.State() != Ready {
		t.Errorf("a log in memory isn't shown at once: state %d", m.State())
	}
}

func TestPendingJobLoadsOnceDone(t *testing.T) {
	f := newFake()
	f.logs[runningJob] = core.Log{Lines: []core.LogLine{{Text: "0 issues.", Step: 2}}}
	m := newView(t, f, 80, 12)
	run(m, m.Show(running(), false, Hints{}))
	j := running()
	j.Steps[1].Status, j.Steps[1].Conclusion = core.RunCompleted, core.ConclusionSuccess
	j.Steps[1].CompletedAt = at(time.Second)
	run(m, m.Show(j, false, Hints{}))
	if m.State() != Pending || !strings.Contains(text(m), "✓ Run golangci-lint") {
		t.Errorf("the steps didn't move on: state %d\n%s", m.State(), text(m))
	}
	j.Status, j.Conclusion = core.RunCompleted, core.ConclusionSuccess
	run(m, m.Show(j, false, Hints{}))
	if m.State() != Ready || m.Lines() != 1 {
		t.Errorf("the log didn't load once the job was done: state %d", m.State())
	}
}

func TestTruncatedNotice(t *testing.T) {
	f := newFake()
	lg := testLog()
	lg.Truncated = true
	f.logs[failedJob] = lg
	m := newView(t, f, 80, 12)
	run(m, m.Show(failed(), false, Hints{}))
	if s := text(m); !strings.Contains(s, "Only the end of this log: it is too large to read whole. o opens it on GitHub.") {
		t.Errorf("a truncated log doesn't say so:\n%s", s)
	}
	assertFits(t, m.View(), 80, 12)
}

func TestMessagesOfOtherViewsAreIgnored(t *testing.T) {
	f := newFake()
	m := newView(t, f, 80, 12)
	other := newView(t, f, 80, 12)
	run(m, m.Show(failed(), false, Hints{}))
	*m, _ = m.Update(logMsg{id: other.id, jobID: failedJob, err: errBoom})
	*m, _ = m.Update(restMsg{id: other.id, seq: m.seq})
	if m.State() != Ready {
		t.Error("the messages of another view changed this one")
	}
}

func TestClear(t *testing.T) {
	m := newView(t, newFake(), 40, 4)
	run(m, m.Show(failed(), false, Hints{}))
	m.Clear()
	if _, ok := m.Job(); ok || m.JobID() != 0 || m.Title() != "" {
		t.Error("a cleared view still shows its job")
	}
}

func TestView(t *testing.T) {
	tests := []struct {
		name string
		job  core.Job
		w, h int
	}{
		{"failed", failed(), 80, 12},
		{"running", running(), 60, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newView(t, newFake(), tt.w, tt.h)
			run(m, m.Show(tt.job, false, Hints{}))
			v := m.View()
			assertFits(t, v, tt.w, tt.h)
			golden.RequireEqual(t, v)
		})
	}
}

func TestViewFitsAnySize(t *testing.T) {
	for _, size := range [][2]int{{0, 0}, {1, 1}, {10, 3}, {200, 50}} {
		for _, j := range []core.Job{failed(), running()} {
			m := newView(t, newFake(), 80, 12)
			run(m, m.Show(j, false, Hints{}))
			m.SetSize(size[0], size[1])
			if size[1] == 0 {
				continue
			}
			assertFits(t, m.View(), size[0], size[1])
		}
	}
}

func TestAnnotations(t *testing.T) {
	f := newFake()
	m := newView(t, f, 80, 20)
	run(m, m.Show(failed(), false, Hints{SHA: "f00d"}))
	s := text(m)
	for _, want := range []string{"Annotations 3 A to pick one", "✗ tea_test.go:54 want a frame, got none", "! key.go:12 unused: x is unused", "✗ Process completed with exit code 1."} {
		if !strings.Contains(s, want) {
			t.Errorf("the view lacks %q:\n%s", want, s)
		}
	}
	assertFits(t, m.View(), 80, 20)
	m.Focus()
	if got := keys(m, "A"); len(got) != 0 || !m.OnAnnotations() || m.view.Focused() {
		t.Fatal("A didn't give the keys to the annotations")
	}
	// Folding every step works from the annotations, which keep the keys.
	before := m.view.View()
	if got := keys(m, "*"); len(got) != 0 || !m.OnAnnotations() || m.view.Focused() || m.view.View() == before {
		t.Error("* didn't fold the steps of the log from the annotations")
	}
	keys(m, "*")
	got := keys(m, "j", "enter")
	want := ui.OpenFileMsg{Repo: repo, Path: "key.go", Ref: "f00d", Line: 12}
	if len(got) != 1 || got[0] != want {
		t.Errorf("enter sent %v, want %v", got, want)
	}
	if got := keys(m, "j", "enter"); len(got) != 0 {
		t.Errorf("enter on an annotation of the workflow sent %v", got)
	}
	keys(m, "A")
	if m.OnAnnotations() || !m.view.Focused() {
		t.Error("A didn't give the keys back to the log")
	}
	// Blurred and focused again, the log keeps the keys.
	m.Blur()
	m.Focus()
	if m.OnAnnotations() {
		t.Error("the annotations took the keys back")
	}
}

func TestAnnotationsAreReadOnlyForFailedJobs(t *testing.T) {
	f := newFake()
	f.logs[runningJob] = testLog()
	m := newView(t, f, 80, 20)
	j := running()
	j.Status, j.Conclusion = core.RunCompleted, core.ConclusionSuccess
	run(m, m.Show(j, false, Hints{}))
	run(m, m.Show(failed(), false, Hints{NoAnnotations: true}))
	if len(f.noteReads) != 0 || strings.Contains(text(m), "Annotations") {
		t.Errorf("read annotations of %v, want none", f.noteReads)
	}
	// From memory, with the log.
	f.cachedNotes[failedJob], f.cached[failedJob] = true, true
	m = newView(t, f, 80, 20)
	if cmd := m.Show(failed(), false, Hints{}); cmd != nil || !strings.Contains(text(m), "Annotations 3") {
		t.Error("the annotations in memory didn't show at once")
	}
}

func TestAnnotationsRetry(t *testing.T) {
	f := newFake()
	f.notesErr = errBoom
	m := newView(t, f, 80, 20)
	run(m, m.Show(failed(), false, Hints{}))
	if s := text(m); !strings.Contains(s, "Couldn't load the annotations: boom") {
		t.Fatalf("the view doesn't say the annotations failed:\n%s", s)
	}
	f.notesErr = nil
	run(m, m.Retry())
	if s := text(m); !strings.Contains(s, "Annotations 3") {
		t.Errorf("retry didn't read the annotations again:\n%s", s)
	}
}

func TestKeys(t *testing.T) {
	m := newView(t, newFake(), 80, 20)
	run(m, m.Show(failed(), false, Hints{}))
	m.Focus()
	names := func() string {
		bs := m.Keys()
		out := make([]string, 0, len(bs))
		for _, b := range bs {
			out = append(out, b.Help().Key+" "+b.Help().Desc)
		}
		return strings.Join(out, ", ")
	}
	if got := names(); got != "space fold, * fold all, e next error, / search, A annotations" {
		t.Errorf("keys of the log %q", got)
	}
	keys(m, "A")
	if got := names(); got != "↑/k up, ↓/j down, ↵ open file, * fold all, A log" {
		t.Errorf("keys of the annotations %q", got)
	}
	if len(m.FullHelp()) == 0 {
		t.Error("no full help")
	}
}
