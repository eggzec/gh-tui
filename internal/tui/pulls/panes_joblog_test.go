package pulls

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// logChecks reads the log of a job from a fixed list of lines and counts the
// reads, as the service that keeps what it read would see them.
type logChecks struct {
	*fakeChecks
	lines []core.LogLine
	reads atomic.Int32
	// logErr is what a read of the log answers instead of its lines.
	logErr error
	// jobs is what a read of the jobs of the run answers, and jobReads counts
	// those reads.
	jobs     core.Page[core.Job]
	jobReads atomic.Int32
	// partial is the log of a running job as the service has read it, and kept
	// says it has.
	partial core.PartialLog
	kept    bool
	// watches counts the starts of a watch of the log, and stops its ends.
	watches, stops atomic.Int32
}

func (l *logChecks) Log(context.Context, core.RepoRef, int64) (core.Log, error) {
	l.reads.Add(1)
	if l.logErr != nil {
		return core.Log{}, l.logErr
	}
	return core.Log{Lines: l.lines}, nil
}

func (l *logChecks) AllJobs(context.Context, actionssvc.JobsQuery) (core.Page[core.Job], error) {
	l.jobReads.Add(1)
	return l.jobs, nil
}

func (l *logChecks) CachedPartialLog(core.RepoRef, int64) (core.PartialLog, bool) {
	return l.partial, l.kept
}

func (l *logChecks) PartialLog(context.Context, core.RepoRef, int64) (core.PartialLog, error) {
	l.reads.Add(1)
	l.kept = true
	return l.partial, nil
}

func (l *logChecks) WatchLog(core.RepoRef, int64, int64) func() {
	l.watches.Add(1)
	return func() { l.stops.Add(1) }
}

// failedLog is the log of a job whose test failed, with some noise around it.
func failedLog() []core.LogLine {
	lines := make([]core.LogLine, 0, 12)
	for i := range 6 {
		lines = append(lines, core.LogLine{Text: fmt.Sprintf("step %d: setting things up", i), Step: 1})
	}
	lines = append(lines,
		core.LogLine{Text: "go test ./...", Step: 2},
		core.LogLine{Text: "--- FAIL: TestMerge (0.01s)", Step: 2, Kind: core.LogError},
		core.LogLine{Text: "    merge_test.go:42: got 1, want 2", Step: 2, Kind: core.LogError},
		core.LogLine{Text: "the cache is cold", Step: 2, Kind: core.LogWarning},
		core.LogLine{Text: "FAIL", Step: 2},
	)
	return lines
}

// jobPanes returns a Merge pane over a head with one failed job whose log is
// lines, with the first row hovered, and the service that reads the log.
func jobPanes(t *testing.T) (*panesModal, *logChecks) {
	t.Helper()
	return jobPanesOf(t, completedJob(), &logChecks{fakeChecks: &fakeChecks{}, lines: failedLog()})
}

// jobPanesOf is jobPanes for the job cc, read through svc.
func jobPanesOf(t *testing.T, cc core.CommitCheck, svc *logChecks) (*panesModal, *logChecks) {
	t.Helper()
	m, _ := mergePanes(t, nil)
	cc.Workflow, cc.RunNumber = "CI", 1
	c := &checksFake{kept: map[string]core.CommitChecks{"abc123": {SHA: "abc123", Total: 1, Runs: []core.CommitCheck{cc}}}}
	m.checks = svc
	m.merge.watch = (&commitWatches{}).watch
	withChecks(m, c)
	// The preview shows beside the list of a pane that is zoomed.
	m.zoom = true
	m.View()
	return m, svc
}

// hoverJob moves the Merge cursor to the job row and feeds the model the
// answers of the reads it started.
func hoverJob(t *testing.T, m *panesModal) {
	t.Helper()
	for range 12 {
		if m.hover[2].id == jobRow(5) {
			break
		}
		feedLog(m, m.Update(keyMsg("j")))
	}
	if m.hover[2].id != jobRow(5) {
		t.Fatalf("hovered %q, want the job row", m.hover[2].id)
	}
	feedLog(m, m.Update(keyMsg("k")))
	feedLog(m, m.Update(keyMsg("j")))
}

// feedLog hands the model the log that cmd read, if it read one.
func feedLog(m *panesModal, cmd tea.Cmd) {
	if msg, ok := find[mergeLogMsg](cmd); ok {
		m.Update(msg)
	}
}

func TestHoveringAJobReadsItsLogOnce(t *testing.T) {
	t.Parallel()
	m, svc := jobPanes(t)
	hoverJob(t, m)
	if got := svc.reads.Load(); got != 1 {
		t.Fatalf("the log was read %d times, want once", got)
	}
	body := ansi.Strip(m.View())
	if !strings.Contains(body, "--- FAIL: TestMerge") {
		t.Errorf("the preview of the job lacks its first error:\n%s", body)
	}

	// Leaving the row and coming back reuses what was read.
	for range 3 {
		feedLog(m, m.Update(keyMsg("k")))
		feedLog(m, m.Update(keyMsg("j")))
	}
	if got := svc.reads.Load(); got != 1 {
		t.Errorf("the log was read %d times after hovering it again, want once", got)
	}
}

// TestJobLogPreview holds the preview of a failed job in both themes.
func TestJobLogPreview(t *testing.T) {
	t.Parallel()
	for name, dark := range map[string]bool{"dark": true, "light": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, _ := jobPanes(t)
			p, err := config.Default().Palette(dark)
			if err != nil {
				t.Fatal(err)
			}
			m.SetTheme(ui.NewTheme(p, dark))
			hoverJob(t, m)
			// 100 columns is the narrowest that has a preview beside the list.
			m.SetSize(100, 30)
			t.Log(ansi.Strip(m.View()))
			golden.RequireEqual(t, m.View())
		})
	}
}
