package actions

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

func TestOpensOnTheFailedJobAndItsError(t *testing.T) {
	f := newFake()
	m, _ := newModal(t, f, wideW, wideH)
	if !m.hasRun || m.run.ID != failedRun {
		t.Fatalf("shows run %d, want the newest, %d", m.run.ID, failedRun)
	}
	if j, _ := m.jobs.selected(); j.ID != ubuntuJob {
		t.Errorf("opens on job %d, want the failed one, %d", j.ID, ubuntuJob)
	}
	if m.log.State() != jobview.Ready || m.log.Errors() != 1 {
		t.Fatalf("log state %d with %d errors, want the log with its error", m.log.State(), m.log.Errors())
	}
	if s := paneText(m, logPane); !strings.Contains(s, "Process completed with exit code 1.") || !strings.Contains(s, "Run go test") {
		t.Errorf("the log doesn't show the failed step's error:\n%s", s)
	}
	if runs, jobs, logs := f.counts(); runs != 1 || jobs != 1 || logs != 1 {
		t.Errorf("opening read %d pages of runs, %d of jobs and %d logs, want one each", runs, jobs, logs)
	}
	if q := f.queries[0]; q.Filter != (core.RunFilter{}) || q.Repo != repo {
		t.Errorf("first query %+v, want every run of %s", q, repo)
	}
}

func TestJobsAndLogFollowTheCursors(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("j")
	if m.run.ID != runningRun || len(m.jobs.items) != 1 || m.jobs.items[0].ID != runLintJob {
		t.Fatalf("after j the run is %d with %v, want %d with its job", m.run.ID, m.jobs.items, runningRun)
	}
	if m.log.State() != jobview.Pending {
		t.Errorf("the log of a job in progress is in state %d, want pending", m.log.State())
	}
	// Back on the failed run, its jobs and log show from memory at once.
	// Its jobs are read again, which the service answers from memory once
	// the run completed; the log isn't.
	h.hold = func(msg tea.Msg) bool { _, ok := msg.(restMsg); return ok }
	h.keys("k")
	if len(m.jobs.items) != 4 || m.log.State() != jobview.Ready {
		t.Errorf("the failed run shows %d jobs and log state %d, want them from memory", len(m.jobs.items), m.log.State())
	}
	h.release()
	if _, jobs, logs := f.counts(); jobs != 3 || logs != 1 {
		t.Errorf("reads of jobs %d and logs %d, want 3 and 1", jobs, logs)
	}
	if m.log.State() != jobview.Ready {
		t.Errorf("log state %d, want the log from memory", m.log.State())
	}
	h.keys("tab", "j")
	if j, _ := m.jobs.selected(); j.ID != macosJob || m.log.JobID() != macosJob {
		t.Errorf("after j in the jobs the job is %d and the log's %d, want %d", j.ID, m.log.JobID(), macosJob)
	}
}

func TestPanesAndBack(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	steps := []struct {
		key  string
		want pane
	}{
		{"tab", jobsPane}, {"tab", logPane}, {"tab", runsPane}, {"shift+tab", logPane},
		{"h", jobsPane}, {"h", runsPane}, {"h", runsPane}, {"l", jobsPane}, {"l", logPane}, {"l", logPane},
		{"esc", jobsPane}, {"esc", runsPane}, {"enter", jobsPane}, {"enter", logPane},
	}
	for _, s := range steps {
		h.keys(s.key)
		if m.focus != s.want {
			t.Fatalf("after %s the focus is on pane %d, want %d", s.key, m.focus, s.want)
		}
	}
	if !m.log.Focused() || m.runs.Focused() {
		t.Error("the log isn't the one focused bubble")
	}
	h.keys("esc", "esc")
	if len(h.take()) != 0 {
		t.Error("the modal closed before the runs")
	}
	h.keys("esc")
	if got := h.take(); len(got) != 1 || got[0] != (ui.CloseModalMsg{Modal: m}) {
		t.Errorf("esc from the runs sent %v, want the modal closed", got)
	}
	if m.ctx.Err() == nil {
		t.Error("the modal's reads outlive it")
	}
}

func TestEnterInTheLogFolds(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	h.keys("tab", "tab", "g")
	before := paneText(m, logPane)
	h.keys("enter")
	if m.focus != logPane || paneText(m, logPane) == before {
		t.Errorf("enter in the log didn't fold the step under the cursor")
	}
}

func TestZoom(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	h.keys("tab", "tab", "z")
	if !m.zoom || m.paneWidth(logPane) != wideW {
		t.Fatalf("z left the log %d wide, want the modal's %d", m.paneWidth(logPane), wideW)
	}
	if s := screen(m); strings.Contains(s, "Jobs") || strings.Contains(s, "CI #4812") {
		t.Errorf("the zoomed log shows other panes:\n%s", s)
	}
	// The back key unzooms before it steps back.
	h.keys("esc")
	if m.zoom || m.focus != logPane {
		t.Errorf("esc: zoom %v on pane %d, want the log unzoomed", m.zoom, m.focus)
	}
	h.keys("z", "z")
	if m.zoom {
		t.Error("z twice left the pane zoomed")
	}
}

func TestTabs(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	steps := []struct {
		key    string
		tab    int
		filter core.RunFilter
		runs   int
	}{
		{"]", 1, core.RunFilter{Status: "failure"}, 1},
		{"]", 2, core.RunFilter{Status: "in_progress"}, 1},
		{"]", 3, core.RunFilter{Actor: "drew"}, 2},
		{"]", 0, core.RunFilter{}, 4},
		{"[", 3, core.RunFilter{Actor: "drew"}, 2},
	}
	for _, s := range steps {
		h.keys(s.key)
		names, active := m.Tabs()
		if active != s.tab || len(names) != 4 {
			t.Fatalf("after %s the tab is %d of %v, want %d", s.key, active, names, s.tab)
		}
		if q := f.queries[len(f.queries)-1]; q.Filter != s.filter {
			t.Errorf("after %s the runs are read with %+v, want %+v", s.key, q.Filter, s.filter)
		}
		if m.runs.Len() != s.runs {
			t.Errorf("after %s the tab shows %d runs, want %d", s.key, m.runs.Len(), s.runs)
		}
	}
	// A new list shows the jobs of its own first run.
	if m.run.Actor != "drew" || m.run.ID != failedRun {
		t.Errorf("Mine shows run %d of %s", m.run.ID, m.run.Actor)
	}
}

func TestTabsWithoutAViewer(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH, WithViewer(nil))
	if names, _ := m.Tabs(); !slices.Equal(names, []string{"All", "Failing", "Running"}) {
		t.Errorf("tabs %v, want no Mine without a viewer", names)
	}
	h.keys("[")
	if _, active := m.Tabs(); active != 2 {
		t.Errorf("[ from All shows tab %d, want Running", active)
	}
}

func TestFilterStep(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("f")
	if m.filterStep == nil || m.filterStep.form == nil || f.wfReads != 1 {
		t.Fatalf("f didn't open the filter over the workflows (%d reads)", f.wfReads)
	}
	if s := screen(m); !strings.Contains(s, "Filter runs") || !strings.Contains(s, "Workflow") {
		t.Errorf("the filter step shows:\n%s", s)
	}
	form := m.filterStep.form
	form.SetQuery(`workflow:CI branch:main event:push status:failure actor:@me`)
	h.send(filterform.AppliedMsg{ID: form.ID(), Values: form.Values()})
	want := core.RunFilter{WorkflowID: 1, Branch: "main", Event: "push", Status: "failure", Actor: me}
	if m.filterStep != nil || m.filter != want {
		t.Fatalf("applied filter %+v (step open %v), want %+v", m.filter, m.filterStep != nil, want)
	}
	if q := f.queries[len(f.queries)-1].Filter; q.Actor != "drew" || q.WorkflowID != 1 || q.Status != "failure" {
		t.Errorf("runs read with %+v, want @me as the viewer", q)
	}
	if _, active := m.Tabs(); active != -1 {
		t.Errorf("a filter of more than a tab shows tab %d, want none", active)
	}
	if s := paneText(m, runsPane); !strings.Contains(s, "CI #4812") {
		t.Errorf("the filtered runs:\n%s", s)
	}

	// The form opens on the filter, and the workflows are read once.
	h.keys("f")
	if got := m.filterStep.form.Query(); got != "workflow:CI branch:main event:push status:failure actor:@me" {
		t.Errorf("the form opens on %q", got)
	}
	if f.wfReads != 1 {
		t.Errorf("the workflows were read %d times, want once", f.wfReads)
	}
	// esc closes the step and keeps the filter.
	queries := len(f.queries)
	h.keys("esc")
	if m.filterStep != nil || m.filter != want || len(f.queries) != queries {
		t.Errorf("esc: step open %v, filter %+v, %d new reads", m.filterStep != nil, m.filter, len(f.queries)-queries)
	}
	// A tab keeps the rest of the filter.
	h.keys("]")
	if m.filter != (core.RunFilter{WorkflowID: 1, Branch: "main", Event: "push"}) {
		t.Errorf("] from a filter shows %+v, want All with its workflow, branch and event", m.filter)
	}
	if got := m.filterText(); got != "workflow:CI branch:main event:push" {
		t.Errorf("the title of the runs shows %q", got)
	}
}

// Workflows an earlier session kept open the form at once, and are read
// again past the kept ones.
func TestKeptWorkflowsAreReadAgain(t *testing.T) {
	f := newFake()
	f.keptWorkflows = true
	m, h := newModal(t, f, wideW, wideH)
	h.keys("f")
	if m.filterStep == nil || m.filterStep.form == nil {
		t.Fatal("f didn't open the filter over the kept workflows")
	}
	if !slices.Equal(f.wfAgain, []bool{false, true}) {
		t.Errorf("workflows read with Again %v, want the kept ones read again", f.wfAgain)
	}
	h.keys("esc", "f")
	if f.wfReads != 2 {
		t.Errorf("the workflows were read %d times, want twice", f.wfReads)
	}
}

func TestFilterWaitsForTheWorkflows(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	h.hold = func(msg tea.Msg) bool { _, ok := msg.(workflowsMsg); return ok }
	h.keys("f")
	if m.filterStep == nil || m.filterStep.form != nil || !strings.Contains(screen(m), "Loading the workflows") {
		t.Fatalf("the filter doesn't wait for the workflows:\n%s", screen(m))
	}
	// esc leaves while it waits.
	h.keys("esc")
	if m.filterStep != nil {
		t.Fatal("esc didn't close the waiting filter")
	}
	h.release()
	if m.filterStep != nil || !m.workflows.loaded {
		t.Errorf("the workflows reopened the filter, or weren't kept")
	}
}

func TestLogStates(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		state jobview.State
		text  string
	}{
		{"pending", fmt.Errorf("log: %w", core.ErrLogPending), jobview.Pending, "The job hasn't started yet."},
		{"expired", fmt.Errorf("log: %w", core.ErrLogExpired), jobview.Expired, "GitHub no longer keeps this log."},
		{"failed", errors.New("boom"), jobview.Failed, "✗ Something went wrong · r to retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.logErrs[ubuntuJob] = tt.err
			m, _ := newModal(t, f, wideW, wideH)
			if m.log.State() != tt.state {
				t.Fatalf("log state %d, want %d", m.log.State(), tt.state)
			}
			if s := paneText(m, logPane); !strings.Contains(s, tt.text) {
				t.Errorf("the log pane lacks %q:\n%s", tt.text, s)
			}
		})
	}
}

// The runs and the log say what went wrong the way the user should read
// it, without the error's chain, request or status code.
func TestErrorWords(t *testing.T) {
	// The runs pane is narrow, so it cuts the words and keeps the hint,
	// and the log puts the hint on a line of its own where it doesn't fit
	// after the words.
	tests := []struct {
		name       string
		err        error
		want, runs string
	}{
		{"offline", fmt.Errorf("list runs: github: GET /repos/o/r/actions/runs: %w", core.ErrOffline),
			"✗ Can't reach GitHub · r to retry", "✗ Can't reach GitHub · r to retry"},
		{"forbidden", fmt.Errorf("list runs: github: 403 Forbidden: %w", core.ErrForbidden),
			"✗ You don't have access to charmbracelet/bubbletea o to open on GitHub", "✗ You don't have ac… · o to open on GitHub"},
		{"internal", errors.New("list runs: github: decode: unexpected EOF"),
			"✗ Something went wrong. Details", "✗ Something went wrong. Deta… · r to retry"},
	}
	voice := WithVoice(ui.NewVoice(config.Default().Keys, "/var/log/gh-tui.log"))
	clean := func(t *testing.T, pane, s, want string) {
		t.Helper()
		if s = strings.Join(strings.Fields(s), " "); !strings.Contains(s, want) || strings.Contains(s, "github:") || strings.Contains(s, "403") {
			t.Errorf("the %s pane = %q, want %q", pane, s, want)
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.logErrs[ubuntuJob] = tt.err
			m, _ := newModal(t, f, wideW, wideH, voice)
			clean(t, "log", paneText(m, logPane), tt.want)

			f = newFake()
			f.runsErr = tt.err
			m, _ = newModal(t, f, wideW, wideH, voice)
			clean(t, "runs", paneText(m, runsPane), tt.runs)
		})
	}
}

func TestLogRetry(t *testing.T) {
	f := newFake()
	f.logErrs[ubuntuJob] = errors.New("boom")
	m, h := newModal(t, f, wideW, wideH)
	delete(f.logErrs, ubuntuJob)
	h.keys("tab", "tab", "r")
	if m.log.State() != jobview.Ready {
		t.Errorf("r left the log in state %d, want it read again", m.log.State())
	}
}

func TestLogTruncated(t *testing.T) {
	f := newFake()
	lg := testLog()
	lg.Truncated = true
	f.logs[ubuntuJob] = lg
	m, _ := newModal(t, f, wideW, wideH)
	if s := paneText(m, logPane); !strings.Contains(s, "Only the end of this log") {
		t.Errorf("a truncated log doesn't say so:\n%s", s)
	}
	if n := strings.Count(m.log.View(), "\n") + 1; n != m.bodyHeight() {
		t.Errorf("the log with its notice is %d high, want %d", n, m.bodyHeight())
	}
}

func TestPendingLogShowsTheSteps(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("j")
	s := paneText(m, logPane)
	for _, want := range []string{"The log is available when the job finishes.", "✓ Set up job 2s", "◐ Run golangci-lint 1m 18s", "○ Post Run actions/checkout@v4 queued"} {
		if !strings.Contains(s, want) {
			t.Errorf("the log of a running job lacks %q:\n%s", want, s)
		}
	}
	if _, _, logs := f.counts(); logs != 1 {
		t.Errorf("a running job's log was asked for: %d reads", logs)
	}
}

func TestRerunFailedJobs(t *testing.T) {
	f := newFake()
	fl := &follows{}
	m, h := newModal(t, f, wideW, wideH, WithFollow(fl.follow))
	h.keys("ctrl+r")
	if got := lastLine(m); !strings.HasPrefix(got, "Re-run 1 failed job of CI #4812?") {
		t.Fatalf("ctrl+r asks %q", got)
	}
	h.keys("n")
	if m.ask != nil || len(f.sent) != 0 {
		t.Fatal("n didn't drop the re-run")
	}

	h.hold = func(msg tea.Msg) bool { _, ok := msg.(ui.DoneMsg); return ok }
	h.keys("ctrl+r", "y")
	// The change shows before GitHub answers.
	if m.run.Status != core.RunQueued || m.jobs.items[1].Status != core.RunQueued || m.jobs.items[0].Status != core.RunCompleted {
		t.Errorf("after y the run is %s and its jobs %s, %s; want the run and its failed job queued",
			m.run.Status, m.jobs.items[0].Status, m.jobs.items[1].Status)
	}
	if s := paneText(m, runsPane); !strings.Contains(s, "○ CI #4812") {
		t.Errorf("the runs don't show the re-run queued:\n%s", s)
	}
	if !slices.Equal(fl.started, []int64{failedRun}) {
		t.Errorf("follows %v, want the re-run followed", fl.started)
	}
	h.release()
	if !slices.Equal(f.sent, []string{"rerun failed"}) {
		t.Errorf("sent %v", f.sent)
	}
	done := h.take()
	if len(done) != 1 || done[0].(ui.DoneMsg).Err != nil || done[0].(ui.DoneMsg).From != Title {
		t.Errorf("the change reported %v", done)
	}
	if f.runReads != 0 {
		t.Errorf("the run was read %d times, while the sync engine follows it", f.runReads)
	}
}

func TestOtherKeysLeaveTheQuestionOpen(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("R", "j", "tab", "q", "enter")
	if got := lastLine(m); !strings.HasPrefix(got, "Re-run all jobs of CI #4812?") || m.focus != runsPane {
		t.Fatalf("after other keys the modal shows %q with the focus on %d", got, m.focus)
	}
	h.take()
	h.keys("esc")
	if m.ask != nil || len(f.sent) != 0 || slices.Contains(h.take(), tea.Msg(ui.CloseModalMsg{Modal: m})) {
		t.Errorf("esc left the question %v and sent %v; want it dropped and the modal open", m.ask, f.sent)
	}
}

func TestRerunReadsTheRunWithoutASyncEngine(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("R")
	if got := lastLine(m); !strings.HasPrefix(got, "Re-run all jobs of CI #4812?") {
		t.Fatalf("R asks %q", got)
	}
	h.keys("y")
	if !slices.Equal(f.sent, []string{"rerun"}) || f.runReads != 1 {
		t.Errorf("sent %v and read the run %d times, want it read once after", f.sent, f.runReads)
	}
}

func TestRefusedChangeRollsBack(t *testing.T) {
	f := newFake()
	refused := &core.RefusedError{Action: "run can't be re-run", Reason: "This workflow run is too old to re-run."}
	f.refuse = fmt.Errorf("re-run failed jobs of run 4812: %w", refused)
	m, h := newModal(t, f, wideW, wideH)
	h.keys("ctrl+r", "y")
	if m.run.Status != core.RunCompleted || m.run.Conclusion != core.ConclusionFailure || m.jobs.items[1].Conclusion != core.ConclusionFailure {
		t.Errorf("after the refusal the run is %s %s, want it as it was", m.run.Status, m.run.Conclusion)
	}
	got := h.take()
	if len(got) != 1 {
		t.Fatalf("the refusal reported %v", got)
	}
	done := got[0].(ui.DoneMsg)
	// The app's toast reads "Couldn't <what>: <err>".
	if done.What != "re-run the failed jobs of CI #4812" || done.Err == nil || done.Err.Error() != refused.Error() {
		t.Errorf("toast of %q with %v, want GitHub's reason alone", done.What, done.Err)
	}
}

func TestCancelARunningRun(t *testing.T) {
	f := newFake()
	fl := &follows{}
	m, h := newModal(t, f, wideW, wideH, WithFollow(fl.follow))
	h.keys("ctrl+r")
	if got := lastLine(m); !strings.Contains(got, "has no failed jobs") && !strings.HasPrefix(got, "Re-run") {
		t.Fatalf("ctrl+r on the failed run: %q", got)
	}
	h.keys("n", "j", "ctrl+r")
	if got := lastLine(m); got != "lint #4810 is still running. x cancels it." {
		t.Errorf("ctrl+r on a running run tells %q", got)
	}
	h.keys("x")
	if got := lastLine(m); !strings.HasPrefix(got, "Cancel lint #4810?") {
		t.Fatalf("x asks %q", got)
	}
	h.keys("y")
	if m.run.Status != core.RunCancelling || !slices.Equal(f.sent, []string{"cancel"}) {
		t.Errorf("after y the run is %s and sent %v", m.run.Status, f.sent)
	}
	if s := paneText(m, runsPane); !strings.Contains(s, "cancelling") {
		t.Errorf("the runs don't show the cancel:\n%s", s)
	}
	// A done run can't be cancelled.
	h.keys("k", "x")
	if got := lastLine(m); got != "CI #4812 isn't running." {
		t.Errorf("x on a done run tells %q", got)
	}
}

func TestRerunAJob(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("J")
	if m.ask != nil {
		t.Fatal("J asked from the runs, where no job is picked")
	}
	h.keys("tab", "j", "J")
	if got := lastLine(m); !strings.HasPrefix(got, "Re-run test (macos-latest, 1.26) of CI #4812?") {
		t.Fatalf("J asks %q", got)
	}
	// Enter isn't a yes.
	h.keys("enter")
	if m.ask == nil || len(f.sent) != 0 {
		t.Fatalf("enter answered the question and sent %v", f.sent)
	}
	h.keys("y")
	if !slices.Equal(f.sent, []string{fmt.Sprintf("rerun job %d", macosJob)}) {
		t.Errorf("sent %v", f.sent)
	}
	if j, _ := m.jobs.selected(); j.Status != core.RunQueued || m.jobs.items[1].Status != core.RunCompleted {
		t.Errorf("the job is %s, want it alone queued", j.Status)
	}
}

// leads bring up the question of each change, and want is what y sends.
var leads = []struct {
	name string
	keys []string
	want string
}{
	{"re-run failed jobs", []string{"ctrl+r"}, "rerun failed"},
	{"re-run all jobs", []string{"R"}, "rerun"},
	{"re-run a job", []string{"tab", "j", "J"}, fmt.Sprintf("rerun job %d", macosJob)},
	{"cancel", []string{"j", "x"}, "cancel"},
}

func TestASecondYesChangesOnce(t *testing.T) {
	for _, tt := range leads {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			m, h := newModal(t, f, wideW, wideH)
			h.keys(tt.keys...)
			if m.ask == nil {
				t.Fatal("asked nothing")
			}
			// The answers arrive before what the first starts runs, as a
			// repeated key does.
			cmds := []tea.Cmd{m.Update(press("y")), m.Update(press("y"))}
			for _, c := range cmds {
				h.run(c)
			}
			if !slices.Equal(f.sent, []string{tt.want}) {
				t.Errorf("sent %v, want %q once", f.sent, tt.want)
			}
		})
	}
}

func TestChangesAskAgain(t *testing.T) {
	// finished makes the run shown done, and restarted runs it again, as
	// a change elsewhere would, which a poll then shows.
	finished := func(m *Modal) { m.run.Status, m.run.Conclusion = core.RunCompleted, core.ConclusionSuccess }
	restarted := func(m *Modal) { m.run.Status, m.run.Conclusion = core.RunQueued, "" }
	tests := []struct {
		lead   int
		meddle func(m *Modal)
		want   string
	}{
		{0, restarted, "CI #4812 changed meanwhile, so nothing was sent."},
		{1, restarted, "CI #4812 changed meanwhile, so nothing was sent."},
		{2, restarted, "test (macos-latest, 1.26) of CI #4812 changed meanwhile, so nothing was sent."},
		{3, finished, "lint #4810 changed meanwhile, so nothing was sent."},
	}
	for _, tt := range tests {
		t.Run(leads[tt.lead].name, func(t *testing.T) {
			f := newFake()
			m, h := newModal(t, f, wideW, wideH)
			h.keys(leads[tt.lead].keys...)
			if m.ask == nil {
				t.Fatal("asked nothing")
			}
			tt.meddle(m)
			h.take()
			h.keys("y")
			want := ui.NotifyMsg{Level: toast.Info, Text: tt.want}
			if got := h.take(); len(f.sent) != 0 || !slices.Contains(got, tea.Msg(want)) {
				t.Errorf("sent %v and showed %v, want only %q", f.sent, got, tt.want)
			}
		})
		t.Run(leads[tt.lead].name+" after access was lost", func(t *testing.T) {
			f := newFake()
			m, h := newModal(t, f, wideW, wideH)
			h.keys(leads[tt.lead].keys...)
			if m.ask == nil {
				t.Fatal("asked nothing")
			}
			h.send(ui.CapsMsg{Repo: repo, Caps: core.RepoCaps{Known: true, Permission: core.PermissionRead}})
			h.take()
			h.keys("y")
			got := h.take()
			if len(f.sent) != 0 || len(got) != 1 {
				t.Fatalf("sent %v and showed %v, want only the refusal", f.sent, got)
			}
			if n, ok := got[0].(ui.NotifyMsg); !ok || !strings.Contains(n.Text, "needs write access") {
				t.Errorf("showed %v, want the refusal", got[0])
			}
		})
	}
}

func TestFollowsTheRunShownWhileItRuns(t *testing.T) {
	f := newFake()
	fl := &follows{}
	m, h := newModal(t, f, wideW, wideH, WithFollow(fl.follow))
	if len(fl.started) != 0 {
		t.Fatalf("follows %v on a done run", fl.started)
	}
	h.keys("j")
	if !slices.Equal(fl.started, []int64{runningRun}) {
		t.Fatalf("follows %v, want the running run", fl.started)
	}
	h.keys("j")
	if !slices.Equal(fl.stopped, []int64{runningRun}) || m.following != 0 {
		t.Errorf("stopped %v on a done run, want the running one stopped", fl.stopped)
	}
	h.keys("k", "esc")
	if !slices.Equal(fl.stopped, []int64{runningRun, runningRun}) {
		t.Errorf("stopped %v, want the run stopped once the modal closed", fl.stopped)
	}
}

func TestPollUpdatesTheRunAndLoadsTheLogOnceDone(t *testing.T) {
	f := newFake()
	fl := &follows{}
	m, h := newModal(t, f, wideW, wideH, WithFollow(fl.follow))
	h.keys("j")
	key := actionssvc.RunSyncKey(repo, runningRun)

	// A step moves on.
	jobs := testJobs()[runningRun]
	jobs[0].Steps[1].Status, jobs[0].Steps[1].Conclusion = core.RunCompleted, core.ConclusionSuccess
	jobs[0].Steps[1].CompletedAt = at(5 * 1e9)
	jobs[0].Steps[2].Status = core.RunInProgress
	f.setJobs(runningRun, jobs)
	h.send(ui.SyncMsg{Key: key})
	if s := paneText(m, logPane); !strings.Contains(s, "✓ Run golangci-lint") || !strings.Contains(s, "◐ Post Run") {
		t.Errorf("the poll didn't move the steps on:\n%s", s)
	}
	// Another run's key, or an error, is left alone.
	h.send(ui.SyncMsg{Key: actionssvc.RunSyncKey(repo, failedRun)})
	h.send(ui.SyncMsg{Key: key, Err: errors.New("offline")})

	// The run completes: the log loads, and the run is no longer followed.
	f.logs[runLintJob] = core.Log{Lines: []core.LogLine{{Text: "0 issues.", Kind: core.LogPlain, Step: 2}}}
	run, _ := f.CachedRun(repo, runningRun)
	run.Status, run.Conclusion = core.RunCompleted, core.ConclusionSuccess
	f.setRun(run)
	jobs[0].Status, jobs[0].Conclusion, jobs[0].CompletedAt = core.RunCompleted, core.ConclusionSuccess, at(1e9)
	f.setJobs(runningRun, jobs)
	h.send(ui.SyncMsg{Key: key})
	if m.log.State() != jobview.Ready || m.log.Lines() != 1 {
		t.Errorf("the log didn't load once the job was done: state %d", m.log.State())
	}
	if !slices.Equal(fl.stopped, []int64{runningRun}) {
		t.Errorf("stopped %v, want the completed run stopped", fl.stopped)
	}
	if s := paneText(m, runsPane); !strings.Contains(s, "✓ lint #4810") {
		t.Errorf("the runs don't show the run done:\n%s", s)
	}
}

func TestSyncKeyReloadsTheRuns(t *testing.T) {
	f := newFake()
	_, h := newModal(t, f, wideW, wideH)
	h.send(ui.SyncMsg{Key: actionssvc.SyncKey(repo)})
	if runs, _, _ := f.counts(); runs != 2 {
		t.Errorf("the runs were read %d times, want again after the sync", runs)
	}
	h.send(ui.SyncMsg{Key: actionssvc.SyncKey(core.RepoRef{Owner: "o", Name: "r"})})
	if runs, _, _ := f.counts(); runs != 2 {
		t.Error("another repository's sync read the runs")
	}
}

func TestOpenInTheBrowser(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	h.keys("o", "tab", "o")
	got := h.take()
	want := []tea.Msg{ui.OpenMsg{URL: m.run.URL}, ui.OpenMsg{URL: m.jobs.items[1].URL}}
	if !slices.Equal(got, want) {
		t.Errorf("o sent %v, want %v", got, want)
	}
}

func TestErrorsAreInline(t *testing.T) {
	f := newFake()
	f.jobsErr = errors.New("jobs boom")
	m, h := newModal(t, f, wideW, wideH)
	if s := paneText(m, jobsPane); !strings.Contains(s, "Couldn't load the jobs: jobs boom · r to retry") {
		t.Errorf("the jobs pane:\n%s", s)
	}
	f.jobsErr = nil
	h.keys("tab", "r")
	if !m.jobs.loaded || m.jobs.err != nil {
		t.Errorf("r didn't read the jobs again")
	}

	f = newFake()
	f.runsErr = errors.New("runs boom")
	m, _ = newModal(t, f, wideW, wideH)
	if s := paneText(m, runsPane); !strings.Contains(s, "Something went wrong") {
		t.Errorf("the runs pane:\n%s", s)
	}
}

func TestMessagesOfOtherModalsAreIgnored(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	other, _ := newModal(t, f, wideW, wideH)
	h.send(jobsMsg{id: other.id, runID: failedRun, attempt: 1})
	h.send(ui.DoneMsg{From: "Issues", Err: errors.New("x")})
	if m.log.State() != jobview.Ready || len(m.jobs.items) != 4 {
		t.Error("the messages of another modal changed this one")
	}
}

func TestALogWithoutErrorsOpensOnItsSteps(t *testing.T) {
	f := newFake()
	lg := testLog()
	lg.Lines = slices.DeleteFunc(lg.Lines, func(l core.LogLine) bool { return l.Kind == core.LogError })
	f.logs[macosJob] = lg
	m, h := newModal(t, f, wideW, wideH)
	h.keys("tab", "j")
	s := paneText(m, logPane)
	// The job's steps have no names in the fake, so they are numbered.
	if !strings.Contains(s, "▸ Step 1 ▸ Step 2 ▸ Step 3") || strings.Contains(s, "FAIL: TestProgram") {
		t.Errorf("a log without errors doesn't open folded:\n%s", s)
	}
}

func TestHelpNamesWhatTheKeysDo(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	short := func() string {
		var ks []string
		for _, b := range m.Help().ShortHelp() {
			if b.Enabled() {
				ks = append(ks, b.Help().Key+" "+b.Help().Desc)
			}
		}
		return strings.Join(ks, ", ")
	}
	steps := []struct {
		keys []string
		want string
	}{
		{nil, "↵ jobs, tab pane, z zoom, ^r rerun failed, R rerun all, f filter, o browser"},
		{[]string{"tab"}, "↵ log, tab pane, z zoom, ^r rerun failed, R rerun all, J rerun job, f filter, o browser"},
		{[]string{"tab"}, "space fold, * fold all, e next error, / search, A annotations, tab pane, z zoom, ^r rerun failed, R rerun all, J rerun job, f filter, o browser"},
		{[]string{"A"}, "↑/k up, ↓/j down, ↵ open file, * fold all, A log, tab pane, z zoom, ^r rerun failed, R rerun all, J rerun job, f filter, o browser"},
		{[]string{"A"}, "space fold, * fold all, e next error, / search, A annotations, tab pane, z zoom, ^r rerun failed, R rerun all, J rerun job, f filter, o browser"},
		// The steps of a job in progress don't fold.
		{[]string{"tab", "j", "tab", "tab"}, "tab pane, z zoom, x cancel run, f filter, o browser"},
		{[]string{"x"}, "y yes, n no"},
	}
	for _, s := range steps {
		h.keys(s.keys...)
		if got := short(); got != s.want {
			t.Errorf("after %q the help is %q, want %q", s.keys, got, s.want)
		}
	}
}

func TestRunsWithoutJobsSayWhy(t *testing.T) {
	tests := []struct {
		conclusion core.Conclusion
		want       string
	}{
		{core.ConclusionActionRequired, "Waiting for a maintainer to approve this run · o opens it on GitHub"},
		{core.ConclusionStartupFailure, "This run failed to start, often for an error in its workflow file · o opens it on GitHub"},
		{core.ConclusionSkipped, "This run was skipped, so none of its jobs ran."},
		{core.ConclusionNone, "This run has no jobs yet."},
	}
	for _, tt := range tests {
		t.Run(string(tt.conclusion), func(t *testing.T) {
			f := newFake()
			f.runs[0].Conclusion = tt.conclusion
			f.jobs[failedRun] = nil
			m, _ := newModal(t, f, wideW, wideH)
			if got := m.noJobsText(); got != tt.want {
				t.Errorf("the jobs say %q, want %q", got, tt.want)
			}
			if s := paneText(m, jobsPane); !strings.Contains(s, strings.Fields(tt.want)[0]) {
				t.Errorf("the jobs pane doesn't show why:\n%s", s)
			}
		})
	}
}

func TestAnnotationsOpenTheirFile(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	if s := paneText(m, logPane); !strings.Contains(s, "Annotations 2") || !strings.Contains(s, "✗ tea_test.go:54 want a frame, got none") {
		t.Fatalf("the log pane doesn't show the annotations:\n%s", s)
	}
	if !slices.Equal(f.noteReads, []int64{ubuntuJob}) {
		t.Errorf("read the annotations of %v, want those of the failed job", f.noteReads)
	}
	h.keys("tab", "tab", "A")
	if !m.log.OnAnnotations() {
		t.Fatal("A in the log didn't focus the annotations")
	}
	h.keys("enter")
	want := ui.OpenFileMsg{Repo: repo, Path: "tea_test.go", Ref: "f00dcafe", Line: 54, Return: m}
	if got := h.take(); len(got) != 1 || got[0] != want {
		t.Fatalf("enter sent %v, want the file previewed at the run's commit", got)
	}
	// The exit code is about the workflow, not a file.
	h.keys("j", "enter")
	if got := h.take(); len(got) != 0 {
		t.Errorf("enter on an annotation of the workflow sent %v", got)
	}
	if m.focus != logPane {
		t.Errorf("enter in the annotations moved the focus to pane %d", m.focus)
	}
	h.keys("A")
	if m.log.OnAnnotations() {
		t.Error("A didn't give the keys back to the log")
	}
	// A job that passed has none worth reading.
	h.keys("shift+tab", "j")
	if len(f.noteReads) != 1 {
		t.Errorf("read the annotations of %v, want none of a job that passed", f.noteReads)
	}
}

func TestReopenedRestartsTheTimers(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH, WithFollow(new(follows).follow))
	other, _ := newModal(t, f, wideW, wideH)
	h.keys("j")
	// While a preview hid the modal, its ticks went to the preview, so
	// they stopped.
	m.opts.tick = time.Second
	if m.Update(ui.ReopenedMsg{Modal: other}) != nil || m.ticking {
		t.Error("another modal's reopening changed this one")
	}
	if cmd := m.Update(ui.ReopenedMsg{Modal: m}); cmd == nil || !m.ticking {
		t.Error("reopened didn't start the timers again")
	}
}
