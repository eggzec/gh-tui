package jobview

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
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
		{"running", running(), nil, Pending, "Logs arrive in ~2 MB blocks while the job runs · o to watch live on GitHub ✓ Set up job 2s ◐ Run golangci-lint 1m 18s ○ Complete job queued"},
		{"not yet", failed(), fmt.Errorf("log: %w", core.ErrLogPending), Pending, "The job hasn't started yet."},
		{"expired", failed(), fmt.Errorf("log: %w", core.ErrLogExpired), Expired, "GitHub no longer keeps this log. ✓ Set up job 2s ✗ Run go test ./... 2m 58s"},
		{"failed", failed(), errBoom, Failed, "✗ Something went wrong"},
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

// The pending text points to GitHub only while the key that opens it
// works.
func TestPendingWithoutOpen(t *testing.T) {
	k := testKeys()
	k.Open.SetEnabled(false)
	m := New(t.Context(), newFake(), repo, k, WithClock(func() time.Time { return testNow }))
	m.SetTheme(testTheme())
	m.SetSize(80, 12)
	run(&m, m.Show(running(), false, Hints{}))
	if s := text(&m); !strings.Contains(s, "Logs arrive in ~2 MB blocks while the job runs.") || strings.Contains(s, "live") {
		t.Errorf("the pending text names a key that doesn't work:\n%s", s)
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
	f.cached[failedJob] = true
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
		name    string
		job     core.Job
		partial bool
		w, h    int
	}{
		{"failed", failed(), false, 80, 12},
		{"running", running(), false, 60, 8},
		{"partial", running(), true, 60, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			if tt.partial {
				f.partial[runningJob] = partialLog(2, 5, 1)
			}
			m := newView(t, f, tt.w, tt.h)
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

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, testKeys())
}

// With the ASCII icons the job draws ASCII alone: its steps, the log's
// gutter and folds, and the annotations.
func TestViewASCII(t *testing.T) {
	for _, j := range []core.Job{failed(), running()} {
		f := newFake()
		m := newView(t, f, 80, 20, WithIcons(ui.NewIcons(config.IconsASCII)))
		run(m, m.Show(j, false, Hints{}))
		if v := ansi.Strip(m.View()); strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("job %s: view isn't ASCII:\n%s", j.Name, v)
		}
	}
}

// TestReadLostReadsTheLogAgain checks that a log whose read was lost,
// while a preview hid the view, is read again at once, even one that
// waited for a rest, and that the rest in flight is stale then.
func TestReadLostReadsTheLogAgain(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%v", rest), func(t *testing.T) {
			f := newFake()
			m := newView(t, f, 80, 12, WithRest(time.Hour))
			_ = m.Show(failed(), rest, Hints{}) // Its answer goes to the preview.
			if m.State() != Loading {
				t.Fatalf("state %d, want loading", m.State())
			}
			seq := m.seq
			run(m, m.ReadLost())
			if m.State() != Ready || !slices.Equal(f.reads, []int64{failedJob}) {
				t.Errorf("read lost left state %d after reads %v, want the log read once", m.State(), f.reads)
			}
			var cmd tea.Cmd
			*m, cmd = m.Update(restMsg{id: m.id, seq: seq})
			if cmd != nil {
				t.Error("the rest from before the view was hidden reads")
			}
			if m.ReadLost() != nil {
				t.Error("read lost reads again a log that loaded")
			}
		})
	}
}
