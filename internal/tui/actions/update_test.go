package actions

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestOpensOnTheFailedJobAndItsError(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	if !m.hasRun || m.run.ID != failedRun {
		t.Fatalf("shows run %d, want the newest, %d", m.run.ID, failedRun)
	}
	if j, _ := m.jobs.selected(); j.ID != ubuntuJob {
		t.Errorf("opens on job %d, want the failed one, %d", j.ID, ubuntuJob)
	}
	if h.log != nil {
		t.Fatal("the log opened before a job was picked")
	}
	l := h.openLog(t)
	if l.log.State() != jobview.Ready || l.log.Errors() != 1 {
		t.Fatalf("log state %d with %d errors, want the log with its error", l.log.State(), l.log.Errors())
	}
	if s := logText(l); !strings.Contains(s, "Process completed with exit code 1.") || !strings.Contains(s, "Run go test") {
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
	h.keys("enter", "j") // fold the failed run, then the next run
	if m.run.ID != runningRun || len(m.jobs.items) != 1 || m.jobs.items[0].ID != runLintJob {
		t.Fatalf("after j the run is %d with %v, want %d with its job", m.run.ID, m.jobs.items, runningRun)
	}
	// Back on the failed run, its jobs show from memory at once. They are
	// read again, which the service answers from memory once the run
	// completed.
	h.hold = func(msg tea.Msg) bool { _, ok := msg.(restMsg); return ok }
	h.keys("k")
	if len(m.jobs.items) != 4 {
		t.Errorf("the failed run shows %d jobs, want them from memory", len(m.jobs.items))
	}
	h.release()
	if _, jobs, _ := f.counts(); jobs != 3 {
		t.Errorf("reads of jobs %d, want 3", jobs)
	}
	h.keys("enter", "j", "j") // unfold, into the jobs, down one
	if j, _ := m.jobs.selected(); j.ID != macosJob {
		t.Errorf("after j in the jobs the job is %d, want %d", j.ID, macosJob)
	}
}

func TestPanesAndBack(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	const notes = pane(-1) // stands for the annotations box
	at := func() pane {
		if m.notes {
			return notes
		}
		return m.focus
	}
	steps := []struct {
		key  string
		want pane
	}{
		{"tab", jobsPane}, {"tab", notes}, {"tab", runsPane}, {"shift+tab", notes}, {"shift+tab", jobsPane},
		{"1", runsPane}, {"2", notes}, {"1", runsPane}, {"1", runsPane}, {"2", notes}, {"2", notes},
		{"backspace", runsPane}, {"backspace", runsPane}, {"j", jobsPane},
	}
	for _, s := range steps {
		h.keys(s.key)
		if at() != s.want {
			t.Fatalf("after %s the focus is on pane %d, want %d", s.key, at(), s.want)
		}
	}
	h.keys("backspace", "backspace")
	if len(h.take()) != 0 || m.focus != runsPane {
		t.Error("backspace closed the modal, or left the runs")
	}
	h.keys("esc")
	if got := h.take(); len(got) != 1 || got[0] != (ui.LeaveModalMsg{Modal: m}) {
		t.Errorf("esc from the runs sent %v, want the modal left", got)
	}
	if m.ctx.Err() == nil {
		t.Error("the modal's reads outlive it")
	}
}

func TestLogOverlayOpensAndBackReturns(t *testing.T) {
	t.Parallel()
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	l := h.openLog(t)
	if l.job.ID != ubuntuJob {
		t.Errorf("the log shows job %d, want %d", l.job.ID, ubuntuJob)
	}
	h.keys("backspace")
	if h.log != nil || m.focus != jobsPane {
		t.Errorf("back left the overlay %v on pane %d, want the jobs", h.log != nil, m.focus)
	}
	if len(h.take()) != 0 {
		t.Error("back closed the modal")
	}
}

func TestLogOverlayDismissReturns(t *testing.T) {
	t.Parallel()
	m, h := newModal(t, newFake(), wideW, wideH)
	h.openLog(t)
	h.keys("esc")
	if h.log != nil || m.ctx.Err() != nil || len(h.take()) != 0 {
		t.Error("esc in the log didn't return to the modal alone")
	}
}

func TestLogOverlayStopsWatchingOnClose(t *testing.T) {
	t.Parallel()
	f := newFake()
	_, h := newModal(t, f, wideW, wideH)
	h.openLog(t)
	h.keys("backspace")
	f.mu.Lock()
	defer f.mu.Unlock()
	if n := f.watching[ubuntuJob]; n != 0 {
		t.Errorf("%d watches of the log remain after it closed", n)
	}
}

// The quit intent in the job's log leaves the log, for the app to return to
// the Actions modal, which stays open, and the log's reads end.
func TestLogOverlayQuitReturnsAndStopsWatching(t *testing.T) {
	t.Parallel()
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	l := h.openLog(t)
	cmd, ok := l.Act(ui.ActQuit)
	if !ok || cmd == nil {
		t.Fatalf("Act(quit) = %v, %v, want it taken", cmd, ok)
	}
	if msg := cmd(); msg != (ui.LeaveModalMsg{Modal: l}) {
		t.Errorf("quit sent %v, want the log left", msg)
	}
	if m.ctx.Err() != nil {
		t.Error("quit in the log ended the modal's reads, want the modal kept")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if n := f.watching[ubuntuJob]; n != 0 {
		t.Errorf("%d watches of the log remain after q", n)
	}
}

func TestEnterInTheLogFolds(t *testing.T) {
	t.Parallel()
	_, h := newModal(t, newFake(), wideW, wideH)
	l := h.openLog(t)
	h.keys("g")
	before := logText(l)
	h.keys("enter")
	if h.log != l || logText(l) == before {
		t.Error("enter in the log didn't fold the step under the cursor")
	}
}

func TestLInTheLogUnfolds(t *testing.T) {
	t.Parallel()
	_, h := newModal(t, newFake(), wideW, wideH)
	l := h.openLog(t)
	h.keys("g", "h")
	before := logText(l)
	h.keys("l")
	if h.log != l || logText(l) == before {
		t.Error("l in the log didn't unfold the step under the cursor")
	}
}

// h and l fold the log; the overlay has no panes to move between.
func TestHAndLFoldTheLog(t *testing.T) {
	t.Parallel()
	_, h := newModal(t, newFake(), wideW, wideH)
	l := h.openLog(t)
	for _, k := range []string{"h", "l"} {
		if b, _, ok := uitest.Winner(l.KeyLayers(), k); !ok || b.Help().Desc == "pane left" || b.Help().Desc == "pane right" {
			t.Errorf("the help gives %s to %q, want the log's", k, b.Help().Desc)
		}
		h.keys(k)
		if h.log != l {
			t.Errorf("%s left the log", k)
		}
	}
}

func TestLogOverlayFitsNarrow(t *testing.T) {
	t.Parallel()
	_, h := newModal(t, newFake(), narrowW, narrowH)
	l := h.openLog(t)
	l.SetSize(narrowW, narrowH)
	assertFits(t, l.View(), narrowW, narrowH)
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

// opened returns the filter overlay that the modal asked the app to open.
func opened(tb testing.TB, h *host) *ui.FilterModal {
	tb.Helper()
	fm, _ := openedOver(tb, h)
	return fm
}

// openedOver returns the filter overlay that the modal asked the app to open,
// and the modal it asked to open it over.
func openedOver(tb testing.TB, h *host) (*ui.FilterModal, ui.Modal) {
	tb.Helper()
	for _, msg := range h.take() {
		if o, ok := msg.(ui.OpenModalMsg); ok {
			if fm, ok := o.Modal.(*ui.FilterModal); ok {
				return fm, o.Over
			}
		}
	}
	tb.Fatal("the modal didn't ask for the filter overlay")
	return nil, nil
}

func TestFilterKeepsWhatNoFieldClaims(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	h.run(m.ApplyFilter(filterform.AppliedMsg{
		Query:  "label:bug fix",
		Values: map[string]filterform.Value{ui.SearchField.Key: filterform.TextValue("fix")},
	}))
	if got := m.filter.Words; got != `fix "label:bug"` {
		t.Errorf("words %q, want the search and the unclaimed qualifier", got)
	}
}

func TestFilterOverlay(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("f")
	if f.wfReads != 1 {
		t.Fatalf("f read the workflows %d times, want once", f.wfReads)
	}
	fm, back := openedOver(t, h)
	if fm.Query() != "" {
		t.Errorf("the overlay opens on %q, want no filter", fm.Query())
	}
	if back != ui.Modal(m) {
		t.Errorf("the overlay opens over %v, want the actions modal", back)
	}
	h.run(m.ApplyFilter(filterform.AppliedMsg{
		Query: `workflow:CI branch:main event:push status:failure actor:@me`,
		Values: map[string]filterform.Value{
			fieldWorkflow: filterform.ListValue("CI"),
			fieldBranch:   filterform.ListValue("main"),
			fieldEvent:    filterform.ListValue("push"),
			fieldStatus:   filterform.ListValue("failure"),
			fieldActor:    filterform.ListValue(me),
		},
	}))
	want := core.RunFilter{WorkflowID: 1, Branch: "main", Event: "push", Status: "failure", Actor: me}
	if m.filter != want {
		t.Fatalf("applied filter %+v, want %+v", m.filter, want)
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

	// The overlay opens on the filter, and the workflows are read once.
	h.keys("f")
	if got := opened(t, h).Query(); got != "workflow:CI branch:main event:push status:failure actor:@me" {
		t.Errorf("the overlay opens on %q", got)
	}
	if f.wfReads != 1 {
		t.Errorf("the workflows were read %d times, want once", f.wfReads)
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
	_, h := newModal(t, f, wideW, wideH)
	h.keys("f")
	opened(t, h)
	if !slices.Equal(f.wfAgain, []bool{false, true}) {
		t.Errorf("workflows read with Again %v, want the kept ones read again", f.wfAgain)
	}
	h.keys("f")
	opened(t, h)
	if f.wfReads != 2 {
		t.Errorf("the workflows were read %d times, want twice", f.wfReads)
	}
}

func TestFilterWaitsForTheWorkflows(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	h.hold = func(msg tea.Msg) bool { _, ok := msg.(workflowsMsg); return ok }
	h.keys("f")
	if len(h.take()) != 0 || !m.workflows.wanted {
		t.Fatal("the filter opened before the workflows came, or doesn't wait for them")
	}
	h.hold = nil
	h.release()
	opened(t, h)
	if !m.workflows.loaded || m.workflows.wanted {
		t.Errorf("the workflows weren't kept, or the filter still waits")
	}
}

func TestFilterShowsItWaitsForTheWorkflows(t *testing.T) {
	t.Parallel()
	const text = "Loading the workflows"
	m, h := newModal(t, newFake(), wideW, wideH)
	h.hold = func(msg tea.Msg) bool { _, ok := msg.(workflowsMsg); return ok }
	h.keys("f")
	if !strings.Contains(ansi.Strip(m.View()), text) {
		t.Fatalf("no %q while the filter waits", text)
	}
	h.keys("esc")
	if strings.Contains(ansi.Strip(m.View()), text) {
		t.Errorf("%q still shows after esc", text)
	}
	h.keys("f")
	h.hold = nil
	h.release()
	if strings.Contains(ansi.Strip(m.View()), text) {
		t.Errorf("%q still shows once the workflows came", text)
	}
}

func TestBackCancelsTheWaitForTheWorkflows(t *testing.T) {
	t.Parallel()
	m, h := newModal(t, newFake(), wideW, wideH)
	h.hold = func(msg tea.Msg) bool { _, ok := msg.(workflowsMsg); return ok }
	h.keys("f", "backspace")
	if m.workflows.wanted {
		t.Error("backspace left the filter waiting")
	}
}

// A failed read of the workflows still opens the filter, without workflows
// to choose from, and a later press reads them again.
func TestFilterOpensWhenTheWorkflowsFail(t *testing.T) {
	f := newFake()
	f.wfErr = errors.New("boom")
	m, h := newModal(t, f, wideW, wideH)
	h.keys("f")
	opened(t, h)
	if m.workflows.wanted || m.workflows.loaded {
		t.Errorf("wanted %v, loaded %v after a failed read, want neither", m.workflows.wanted, m.workflows.loaded)
	}
	h.keys("f")
	opened(t, h)
	if f.wfReads != 2 {
		t.Errorf("the workflows were read %d times, want a retry", f.wfReads)
	}
}

// Esc ends the wait for the workflows, so they open no filter on arriving.
func TestEscEndsTheWaitForTheWorkflows(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	h.hold = func(msg tea.Msg) bool { _, ok := msg.(workflowsMsg); return ok }
	h.keys("f")
	h.keys("esc")
	h.hold = nil
	h.release()
	if got := h.take(); len(got) != 0 {
		t.Errorf("workflows arriving after esc sent %v, want nothing", got)
	}
	if !m.workflows.loaded || m.workflows.wanted {
		t.Error("the workflows weren't kept, or the filter still waits")
	}
}

// The tab follows the filter: a status of a tab shows that tab, and a
// branch alone goes back to All.
func TestFilterSyncsTheTab(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	_, before := m.Tabs()
	h.run(m.ApplyFilter(filterform.AppliedMsg{
		Query:  "status:failure",
		Values: map[string]filterform.Value{fieldStatus: filterform.ListValue("failure")},
	}))
	_, failing := m.Tabs()
	if failing == before || failing < 0 {
		t.Errorf("status:failure shows tab %d, was %d, want the Failing tab", failing, before)
	}
	h.run(m.ApplyFilter(filterform.AppliedMsg{
		Query:  "branch:main",
		Values: map[string]filterform.Value{fieldBranch: filterform.ListValue("main")},
	}))
	if _, active := m.Tabs(); active != before {
		t.Errorf("a branch alone shows tab %d, want %d, the tab of no status", active, before)
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
			_, h := newModal(t, f, wideW, wideH)
			l := h.openLog(t)
			if l.log.State() != tt.state {
				t.Fatalf("log state %d, want %d", l.log.State(), tt.state)
			}
			if s := logText(l); !strings.Contains(s, tt.text) {
				t.Errorf("the log pane lacks %q:\n%s", tt.text, s)
			}
		})
	}
}

// The runs, the jobs and the log say what went wrong the way the user
// should read it, without the error's chain, request or status code.
func TestErrorWords(t *testing.T) {
	// The words and their hint are readable whole: the log puts the hint on a
	// line of its own where it doesn't fit, and so does the jobs pane.
	tests := []struct {
		name             string
		err              error
		want, runs, jobs string
	}{
		{"offline", fmt.Errorf("list runs: github: GET /repos/o/r/actions/runs: %w", core.ErrOffline),
			"✗ Can't reach GitHub · r to retry", "✗ Can't reach GitHub · r to retry", "✗ Can't reach GitHub · r to retry"},
		{"forbidden", fmt.Errorf("list runs: github: 403 Forbidden: %w", core.ErrForbidden),
			"✗ You don't have access to charmbracelet/bubbletea · o to open on GitHub",
			"✗ You don't have access to charmbracelet/bubbletea o to open on GitHub",
			"✗ You don't have access to charmbracelet/bubbletea o to open on GitHub"},
		{"not found", fmt.Errorf("list runs: github: 404 Not Found: %w", core.ErrNotFound),
			"✗ charmbracelet/bubbletea doesn't exist or is private.", "✗ charmbracelet/bubbletea doesn't exist or is private.",
			"✗ CI #4812 doesn't exist or is private."},
		{"internal", fmt.Errorf("list runs: github: decode: %s", termtexttest.Hostile),
			"✗ Something went wrong. Details", "✗ Something went wrong. Details are in the log (/var/log/gh-tui.log) · r to retry",
			"✗ Something went wrong. Details are in the log (/var/log/gh-tui.log) · r to retry"},
	}
	voice := WithVoice(ui.NewVoice(config.Default().Keys, "/var/log/gh-tui.log"))
	cleanLog := func(t *testing.T, l *logModal, want string) {
		t.Helper()
		termtexttest.AssertClean(t, l.View(), wideW)
		s := logText(l)
		if !strings.Contains(s, want) {
			t.Errorf("log = %q, want %q", s, want)
		}
		for _, leak := range []string{"github:", "list runs", "GET", "403", "404", "decode"} {
			if strings.Contains(s, leak) {
				t.Errorf("log shows %q: %q", leak, s)
			}
		}
	}
	clean := func(t *testing.T, m *Modal, want string) {
		t.Helper()
		termtexttest.AssertClean(t, m.View(), m.width)
		// The hint follows the words with a dot, or on a line of its own
		// without one, as the room allows.
		norm := strings.NewReplacer("│", " ", "┃", " ", " · ", " ")
		s := strings.Join(strings.Fields(norm.Replace(screen(m))), " ")
		want = strings.Join(strings.Fields(norm.Replace(want)), " ")
		if !strings.Contains(s, want) {
			t.Errorf("%dx%d screen = %q, want %q", m.width, m.height, s, want)
		}
		for _, leak := range []string{"github:", "list runs", "GET", "403", "404", "decode"} {
			if strings.Contains(s, leak) {
				t.Errorf("screen shows %q: %q", leak, s)
			}
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			f.logErrs[ubuntuJob] = tt.err
			_, h := newModal(t, f, wideW, wideH, voice)
			cleanLog(t, h.openLog(t), tt.want)

			// The wide layout shows a failed load of the runs in the
			// annotations box, whole.
			f = newFake()
			f.runsErr = tt.err
			m, _ := newModal(t, f, wideW, wideH, voice)
			clean(t, m, tt.runs)

			f = newFake()
			f.runsErr = tt.err
			m, h = newModal(t, f, narrowW, narrowH, voice)
			h.keys("2")
			clean(t, m, tt.runs)

			f = newFake()
			f.jobsErr = tt.err
			m, h = newModal(t, f, narrowW, narrowH, voice)
			h.keys("tab")
			clean(t, m, tt.jobs)
		})
	}
}

func TestLogRetry(t *testing.T) {
	f := newFake()
	f.logErrs[ubuntuJob] = errors.New("boom")
	_, h := newModal(t, f, wideW, wideH)
	l := h.openLog(t)
	delete(f.logErrs, ubuntuJob)
	h.keys("r")
	if l.log.State() != jobview.Ready {
		t.Errorf("r left the log in state %d, want it read again", l.log.State())
	}
}

func TestLogTruncated(t *testing.T) {
	f := newFake()
	lg := testLog()
	lg.Truncated = true
	f.logs[ubuntuJob] = lg
	_, h := newModal(t, f, wideW, wideH)
	l := h.openLog(t)
	if s := logText(l); !strings.Contains(s, "Only the end of this log") {
		t.Errorf("a truncated log doesn't say so:\n%s", s)
	}
}

func TestPendingLogShowsTheSteps(t *testing.T) {
	f := newFake()
	_, h := newModal(t, f, wideW, wideH)
	h.keys("enter", "j", "j", "enter")
	if h.log == nil {
		t.Fatal("enter on the running job didn't open its log")
	}
	s := logText(h.log)
	for _, want := range []string{"Logs arrive in ~2 MB blocks while the job runs · o to watch live on GitHub", "✓ Set up job 2s", "◐ Run golangci-lint 1m 18s", "○ Post Run actions/checkout@v4 queued"} {
		if !strings.Contains(s, want) {
			t.Errorf("the log of a running job lacks %q:\n%s", want, s)
		}
	}
	if _, _, logs := f.counts(); logs != 0 {
		t.Errorf("a running job's log was asked for: %d reads", logs)
	}
	// The poll reads what GitHub publishes of the log only while the log
	// shows the job.
	if f.watching[runLintJob] != 1 {
		t.Errorf("the log of the running job is watched %d times, want once", f.watching[runLintJob])
	}
	h.keys("esc")
	if h.log != nil || f.watching[runLintJob] != 0 {
		t.Error("the log of a job no longer shown is still watched")
	}
	h.keys("enter")
	if f.watching[runLintJob] != 1 {
		t.Fatal("the log of the running job isn't watched once shown again")
	}
	h.keys("esc", "esc")
	if f.watching[runLintJob] != 0 {
		t.Error("the log of the running job is still watched once the modal closed")
	}
}

// ctrl+r is no key of the modal: it neither refreshes, which is r, nor
// asks to re-run, which is R.
func TestCtrlRDoesNothing(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	reads := f.runReads
	h.keys("ctrl+r")
	if m.ask != nil || f.runReads != reads || len(f.sent) != 0 {
		t.Errorf("ctrl+r asked %v or read the runs again (%d reads, was %d)", m.ask != nil, f.runReads, reads)
	}
}

func TestRerunFailedJobs(t *testing.T) {
	f := newFake()
	fl := &follows{}
	m, h := newModal(t, f, wideW, wideH, WithFollow(fl.follow))
	h.keys("R")
	if got := lastLine(m); !strings.HasPrefix(got, "Re-run 1 failed job of CI #4812?") {
		t.Fatalf("R asks %q", got)
	}
	h.keys("n")
	if m.ask != nil || len(f.sent) != 0 {
		t.Fatal("n didn't drop the re-run")
	}

	h.hold = func(msg tea.Msg) bool { _, ok := msg.(ui.DoneMsg); return ok }
	h.keys("R", "y")
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
	h.keys("E", "j", "tab", "q")
	if got := lastLine(m); !strings.HasPrefix(got, "Re-run all jobs of CI #4812?") || m.focus != runsPane {
		t.Fatalf("after other keys the modal shows %q with the focus on %d", got, m.focus)
	}
	h.take()
	h.keys("esc")
	if m.ask != nil || len(f.sent) != 0 || slices.Contains(h.take(), tea.Msg(ui.CloseModalMsg{Modal: m})) {
		t.Errorf("esc left the question %v and sent %v; want it dropped and the modal open", m.ask, f.sent)
	}
}

// Every change of the modal is a destructive or costly one, so its question
// defaults to no and enter sends nothing.
func TestQuestionsDefaultToNo(t *testing.T) {
	for _, k := range []string{"R", "E"} {
		f := newFake()
		m, h := newModal(t, f, wideW, wideH)
		h.keys(k)
		if m.ask == nil {
			t.Fatalf("%s asked nothing", k)
		}
		if m.ask.Default {
			t.Errorf("%s asks with the default yes", k)
		}
		h.keys("enter")
		if m.ask != nil || len(f.sent) != 0 {
			t.Errorf("after %s and enter the question is %v and sent %v; want it closed and nothing sent", k, m.ask, f.sent)
		}
	}
}

func TestRerunReadsTheRunWithoutASyncEngine(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("E")
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
	h.keys("R", "y")
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
	h.keys("R")
	if got := lastLine(m); !strings.Contains(got, "has no failed jobs") && !strings.HasPrefix(got, "Re-run") {
		t.Fatalf("R on the failed run: %q", got)
	}
	h.keys("n", "enter", "j", "R")
	if got := lastLine(m); got != "lint #4810 is still running. X cancels it." {
		t.Errorf("R on a running run tells %q", got)
	}
	h.keys("X")
	if got := lastLine(m); !strings.HasPrefix(got, "Cancel lint #4810?") {
		t.Fatalf("X asks %q", got)
	}
	h.keys("y")
	if m.run.Status != core.RunCancelling || !slices.Equal(f.sent, []string{"cancel"}) {
		t.Errorf("after y the run is %s and sent %v", m.run.Status, f.sent)
	}
	if s := paneText(m, runsPane); !strings.Contains(s, "cancelling") {
		t.Errorf("the runs don't show the cancel:\n%s", s)
	}
	// A done run can't be cancelled.
	h.keys("k", "X")
	if got := lastLine(m); got != "CI #4812 isn't running." {
		t.Errorf("X on a done run tells %q", got)
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
	// Enter answers the default, which is no: it closes the question and
	// sends nothing.
	h.keys("enter")
	if m.ask != nil || len(f.sent) != 0 {
		t.Fatalf("enter left the question open or sent %v", f.sent)
	}
	h.keys("J")
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
	{"re-run failed jobs", []string{"R"}, "rerun failed"},
	{"re-run all jobs", []string{"E"}, "rerun"},
	{"re-run a job", []string{"j", "j", "J"}, fmt.Sprintf("rerun job %d", macosJob)},
	{"cancel", []string{"enter", "j", "X"}, "cancel"},
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
	h.keys("enter", "j")
	if !slices.Equal(fl.started, []int64{runningRun}) {
		t.Fatalf("follows %v, want the running run", fl.started)
	}
	h.keys("enter", "j")
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
	h.keys("enter", "j", "j", "enter")
	l := h.log
	if l == nil {
		t.Fatal("enter on the running job didn't open its log")
	}
	key := actionssvc.RunSyncKey(repo, runningRun)

	// A step moves on.
	jobs := testJobs()[runningRun]
	jobs[0].Steps[1].Status, jobs[0].Steps[1].Conclusion = core.RunCompleted, core.ConclusionSuccess
	jobs[0].Steps[1].CompletedAt = at(5 * 1e9)
	jobs[0].Steps[2].Status = core.RunInProgress
	f.setJobs(runningRun, jobs)
	h.send(ui.SyncMsg{Key: key})
	if s := logText(l); !strings.Contains(s, "✓ Run golangci-lint") || !strings.Contains(s, "◐ Post Run") {
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
	if l.log.State() != jobview.Ready || l.log.Lines() != 1 {
		t.Errorf("the log didn't load once the job was done: state %d", l.log.State())
	}
	// The modal under the overlay stopped following when it hid; the overlay
	// stops its own follow of the run.
	if len(fl.stopped) == 0 || slices.Max(fl.stopped) != runningRun || slices.Min(fl.stopped) != runningRun {
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
	if s := paneText(m, jobsPane); !strings.Contains(s, "Something went wrong") || !strings.Contains(s, "r to retry") {
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
	if s := screen(m); !strings.Contains(s, "Something went wrong") {
		t.Errorf("the runs pane:\n%s", s)
	}
}

func TestMessagesOfOtherModalsAreIgnored(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	other, _ := newModal(t, f, wideW, wideH)
	h.send(jobsMsg{id: other.id, runID: failedRun, attempt: 1})
	h.send(ui.DoneMsg{From: "Issues", Err: errors.New("x")})
	if len(m.jobs.items) != 4 {
		t.Error("the messages of another modal changed this one")
	}
}

func TestALogWithoutErrorsOpensOnItsSteps(t *testing.T) {
	f := newFake()
	lg := testLog()
	lg.Lines = slices.DeleteFunc(lg.Lines, func(l core.LogLine) bool { return l.Kind == core.LogError })
	f.logs[macosJob] = lg
	_, h := newModal(t, f, wideW, wideH)
	h.keys("tab", "j", "enter")
	if h.log == nil {
		t.Fatal("enter on the job didn't open its log")
	}
	s := logText(h.log)
	// The job's steps have no names in the fake, so they are numbered.
	if !strings.Contains(s, "▸ Step 1 ▸ Step 2 ▸ Step 3") || strings.Contains(s, "FAIL: TestProgram") {
		t.Errorf("a log without errors doesn't open folded:\n%s", s)
	}
}

func TestHelpNamesWhatTheKeysDo(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	short := func() string {
		layers := m.KeyLayers()
		if h.log != nil {
			layers = h.log.KeyLayers()
		}
		var ks []string
		for _, b := range (ui.Hints{Layers: layers}).ShortHelp() {
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
		{nil, "↑/k up, ↓/j down, ↵ fold, f filter, tab pane, R rerun failed, E rerun all, o browser"},
		{[]string{"tab"}, "↑/k up, ↓/j down, ↵ open, J rerun job, tab pane, R rerun failed, E rerun all, o browser"},
		// The log is an overlay with the keys of its own.
		{[]string{"enter"}, "↵ fold, * all, - option: S N, x timestamps, e next error, / search, A annotations"},
		{[]string{"A"}, "* all, - option: S N, ↑/k up, ↓/j down, ↵ open file, A log"},
		{[]string{"A"}, "↵ fold, * all, - option: S N, x timestamps, e next error, / search, A annotations"},
		// The steps of a job in progress don't fold, and have no keys to show.
		{[]string{"backspace", "backspace", "enter", "j", "j", "enter"}, ""},
		{[]string{"backspace", "backspace", "X"}, "y yes, n no, ↵ answer the default"},
	}
	for _, s := range steps {
		h.keys(s.keys...)
		if got := short(); got != s.want {
			t.Errorf("after %q the help is %q, want %q", s.keys, got, s.want)
		}
	}
}

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, newKeyMap(config.Default().Keys))
}

// The layers take a key in the order the modal does: its own keys before
// the pane's, the tabs' ] rather than the next pane's, and the log's
// enter, which folds, rather than the drill of the other panes.
func TestKeyLayersOrder(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	if b, src, _ := uitest.Winner(m.KeyLayers(), "]"); src != "Actions" || b.Help().Desc != "tab" {
		t.Errorf("] reaches %q of %q, want the tabs", b.Help().Desc, src)
	}
	before := m.filter
	h.keys("]")
	if m.filter == before {
		t.Error("] didn't switch the tab")
	}
	if b, src, _ := uitest.Winner(m.KeyLayers(), "enter"); src != "Runs" || b.Help().Desc != "fold" {
		t.Errorf("enter reaches %q of %q on the runs, want the fold", b.Help().Desc, src)
	}
	h.keys("[", "tab")
	if m.focus != jobsPane {
		t.Fatalf("focus %d, want the jobs", m.focus)
	}
	if b, src, _ := uitest.Winner(m.KeyLayers(), "esc"); src != "Actions" || b.Help().Desc != "close" {
		t.Errorf("esc reaches %q of %q on the jobs, want the modal's close", b.Help().Desc, src)
	}
	if b, src, _ := uitest.Winner(m.KeyLayers(), "backspace"); src != "Actions" || b.Help().Desc != "runs" {
		t.Errorf("backspace reaches %q of %q on the jobs, want the step to the runs", b.Help().Desc, src)
	}
	h.keys("backspace")
	if m.focus != runsPane {
		t.Errorf("backspace left the focus on %d, want the runs", m.focus)
	}
}

// TestSharedKeys checks, in each pane, that a key two bindings hold
// reaches the one the modal matches first, and that the help names that
// one too: the filter's f and the panes' h before the log and the lists,
// which page and scroll with them.
func TestSharedKeys(t *testing.T) {
	tab := func(want int) func(*Modal) bool {
		return func(m *Modal) bool { _, active := m.Tabs(); return active == want && m.focus == runsPane }
	}
	asks := func(m *Modal) bool { return m.ask != nil }
	// The filter overlay opens once the workflows are read.
	filters := func(m *Modal) bool { return m.workflows.loaded }
	tests := []struct {
		name string
		to   []string // the keys that reach the pane
		key  string
		desc string // the binding the help names for key, if any
		src  string // the layer the help names it in; the modal's if empty
		ok   func(*Modal) bool
	}{
		{name: "] on the runs", key: "]", desc: "tab", ok: tab(1)},
		{name: "[ on the runs", key: "[", desc: "previous tab", ok: tab(3)},
		{name: "] on the jobs", to: []string{"tab"}, key: "]", desc: "tab",
			ok: func(m *Modal) bool { _, active := m.Tabs(); return active == 1 && m.focus == jobsPane }},
		{name: "tab on the runs", key: "tab", desc: "pane", ok: func(m *Modal) bool { return m.focus == jobsPane }},
		{name: "R on the runs", key: "R", desc: "rerun failed", ok: asks},
		{name: "r on the runs", key: "r", desc: "refresh", ok: func(m *Modal) bool { return m.ask == nil && m.focus == runsPane }},
		// A run still running can't be re-run, so R says why, and the help
		// gives it to no binding it could reach.
		{name: "R on a running run", to: []string{"]", "]"}, key: "R", ok: func(m *Modal) bool { return m.ask == nil && m.notice != "" }},
		{name: "f on the runs", key: "f", desc: "filter", src: "Runs", ok: filters},
		// The filter belongs to the runs: the jobs and the log have no f.
		{name: "f on the jobs", to: []string{"tab"}, key: "f", ok: func(m *Modal) bool { return !m.workflows.loaded && m.focus == jobsPane }},
		{name: "2 on the runs", key: "2", desc: "focus pane", ok: func(m *Modal) bool { return m.notes }},
		{name: "1 on the jobs", to: []string{"tab"}, key: "1", desc: "focus pane", ok: func(m *Modal) bool { return m.focus == runsPane }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, h := newModal(t, newFake(), wideW, wideH)
			h.keys(tt.to...)
			want := tt.src
			if want == "" {
				want = "Actions"
			}
			if b, src, ok := uitest.Winner(m.KeyLayers(), tt.key); ok != (tt.desc != "") || ok && (src != want || b.Help().Desc != tt.desc) {
				t.Errorf("the help gives %s to %q of %q, want %q of %q", tt.key, b.Help().Desc, src, tt.desc, want)
			}
			h.keys(tt.key)
			if !tt.ok(m) {
				t.Errorf("%s didn't reach %q: focus %d, question %v, workflows read %v", tt.key, tt.desc, m.focus, m.ask != nil, m.workflows.loaded)
			}
		})
	}
}

// The filter's keys belong to the runs: F clears what the filter form set
// there, keeping the tab and the commit the modal was opened for, and in
// the log it follows instead, leaving the filter alone.
func TestClearFilterIsTheRunsKey(t *testing.T) {
	form := core.RunFilter{Branch: "main", Event: "push", WorkflowID: 7, HeadSHA: "abc123"}
	m, h := newModal(t, newFake(), wideW, wideH, WithFilter(form))
	h.keys("]")
	tabbed := m.filter
	if tabbed.Status == "" || tabbed.Branch != "main" {
		t.Fatalf("] left the filter at %+v, want a tab and the form's", tabbed)
	}
	if _, src, ok := uitest.Winner(m.KeyLayers(), "F"); !ok || src != "Runs" {
		t.Errorf("F reaches %q on the runs, want the runs' clear filter", src)
	}
	h.keys("tab", "F")
	if m.filter != tabbed {
		t.Errorf("F on the jobs changed the filter to %+v", m.filter)
	}
	h.keys("tab", "1", "F")
	if want := (core.RunFilter{Status: tabbed.Status, HeadSHA: "abc123"}); m.filter != want {
		t.Errorf("F on the runs left the filter at %+v, want %+v", m.filter, want)
	}
	// With nothing of the form left, the key is off.
	if _, _, ok := uitest.Winner(m.KeyLayers(), "F"); ok {
		t.Error("F is offered with no filter of the form set")
	}
	before := m.filter
	h.keys("F")
	if m.filter != before {
		t.Errorf("F changed the filter to %+v with nothing to clear", m.filter)
	}
}

// Alt+f lists the runs without the filter of the form and brings it back,
// keeping the tab and the commit; F clears the kept filter and ends the
// switch, and with no filter there is nothing to switch.
func TestFilterToggleSwitchesTheFormOffAndOn(t *testing.T) {
	form := core.RunFilter{Branch: "main", Event: "push", WorkflowID: 7, HeadSHA: "abc123"}
	m, h := newModal(t, newFake(), wideW, wideH, WithFilter(form))
	h.keys("alt+f")
	if want := cleared(form); m.filter != want {
		t.Fatalf("alt+f left the filter at %+v, want %+v", m.filter, want)
	}
	if kept, off := m.toggle.Kept(); !off || kept != form {
		t.Errorf("the toggle holds %+v, off %v, want the form's filter", kept, off)
	}
	if got := m.paneTitle(runsPane, 80); !strings.Contains(got, "off") {
		t.Errorf("the title %q doesn't say that the filter is off", ansi.Strip(got))
	}
	h.keys("alt+f")
	if m.filter != form || m.toggle.Off() {
		t.Errorf("alt+f again left the filter at %+v, off %v, want %+v", m.filter, m.toggle.Off(), form)
	}
	h.keys("alt+f", "F")
	if want := cleared(form); m.filter != want || m.toggle.Off() {
		t.Errorf("F while off left the filter at %+v, off %v, want %+v", m.filter, m.toggle.Off(), want)
	}
	before := m.filter
	h.take()
	h.keys("alt+f")
	if m.filter != before || m.toggle.Off() {
		t.Errorf("alt+f with no filter changed the filter to %+v", m.filter)
	}
	if len(h.take()) == 0 {
		t.Error("alt+f with no filter said nothing")
	}
}

// F while the filter is off edits the kept filter, and applying it switches
// the filter on.
func TestFilterOpensOnTheKeptFilter(t *testing.T) {
	form := core.RunFilter{Branch: "main"}
	m, h := newModal(t, newFake(), wideW, wideH, WithFilter(form))
	h.keys("alt+f", "f")
	if q := opened(t, h).Query(); !strings.Contains(q, "branch:main") {
		t.Errorf("the overlay opened on %q, want the kept filter", q)
	}
	h.run(m.ApplyFilter(filterform.AppliedMsg{
		Query:  "branch:main",
		Values: map[string]filterform.Value{fieldBranch: filterform.ListValue("main")},
	}))
	if m.toggle.Off() || m.filter.Branch != "main" {
		t.Errorf("applying left the filter at %+v, off %v", m.filter, m.toggle.Off())
	}
}

// Cancelling a run is X: x no longer asks.
func TestLowercaseXDoesNotCancel(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	for range 100 {
		if m.run.ID == runningRun {
			break
		}
		h.keys("down")
	}
	if m.run.ID != runningRun {
		t.Fatalf("no run %v reached by moving down, at %v", runningRun, m.run.ID)
	}
	h.keys("x")
	if m.ask != nil {
		t.Errorf("x asked %q, want nothing", lastLine(m))
	}
	h.keys("X")
	if m.ask == nil {
		t.Error("X didn't ask to cancel the run")
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

func TestJobsNoteSitsUnderTheRunName(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.jobs[failedRun] = nil
	m, _ := newModal(t, f, wideW, wideH)
	for _, w := range []int{80, 120} {
		note, ok := m.jobsNote(w)
		if !ok || len(note) == 0 {
			t.Fatalf("no note at %d columns", w)
		}
		indent := m.st.noGutter + m.jobsIndent()
		for _, l := range note {
			if !strings.HasPrefix(l, indent) {
				t.Errorf("at %d columns the note line %q doesn't start with the job indent %q", w, l, indent)
			}
			if got := ansi.StringWidth(l); got > w {
				t.Errorf("at %d columns the note line is %d cells wide", w, got)
			}
		}
	}
}

func TestAnnotationsOpenTheirFile(t *testing.T) {
	f := newFake()
	_, h := newModal(t, f, wideW, wideH)
	l := h.openLog(t)
	if s := logText(l); !strings.Contains(s, "Annotations 2") || !strings.Contains(s, "✗ tea_test.go:54 want a frame, got none") {
		t.Fatalf("the log doesn't show the annotations:\n%s", s)
	}
	// The pane reads those of every finished job, the failed one first, and
	// the log takes the failed job's from memory.
	reads := []int64{ubuntuJob, 100, 102, 103}
	if !slices.Equal(f.noteReads, reads) {
		t.Errorf("read the annotations of %v, want those of %v once each", f.noteReads, reads)
	}
	h.keys("A")
	if !l.log.OnAnnotations() {
		t.Fatal("A in the log didn't focus the annotations")
	}
	h.keys("enter")
	want := ui.OpenFileMsg{Repo: repo, Path: "tea_test.go", Ref: "f00dcafe", Line: 54}
	if got := h.take(); len(got) != 1 || got[0] != want {
		t.Fatalf("enter sent %v, want the file previewed at the run's commit", got)
	}
	// The exit code is about the workflow, not a file.
	h.keys("j", "enter")
	if got := h.take(); len(got) != 0 {
		t.Errorf("enter on an annotation of the workflow sent %v", got)
	}
	h.keys("A")
	if l.log.OnAnnotations() {
		t.Error("A didn't give the keys back to the log")
	}
	// The log of a job that passed reads none beyond what the pane has.
	h.keys("esc", "j", "enter")
	if !slices.Equal(f.noteReads, reads) {
		t.Errorf("read the annotations of %v, want no more than %v", f.noteReads, reads)
	}
}

func TestReopenedRestartsTheTimers(t *testing.T) {
	f := newFake()
	m, h := newModal(t, f, wideW, wideH, WithFollow(new(follows).follow))
	other, _ := newModal(t, f, wideW, wideH)
	h.keys("enter", "j")
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

// Another modal open in place of this one stops the following of the run
// and the reads of the log of its job, until this one is reopened.
func TestHideStopsTheReads(t *testing.T) {
	f := newFake()
	fl := &follows{}
	_, h := newModal(t, f, wideW, wideH, WithFollow(fl.follow))
	h.keys("enter", "j", "tab", "enter")
	l := h.log
	if l == nil {
		t.Fatal("enter didn't open the log")
	}
	// The modal under the log follows the run too, until the log hides it,
	// so the follows are counted against the stops.
	if f.watching[runLintJob] != 1 || len(fl.started) == 0 || slices.Max(fl.started) != runningRun {
		t.Fatalf("watches %v, follows %v; want the running job's log watched and its run followed", f.watching, fl.started)
	}
	l.Hide()
	if f.watching[runLintJob] != 0 || len(fl.stopped) != len(fl.started) {
		t.Errorf("hidden, watches %v and stopped following %v of %v; want nothing watched or followed", f.watching, fl.stopped, fl.started)
	}
	before := len(fl.started)
	h.send(ui.ReopenedMsg{Modal: l})
	if f.watching[runLintJob] != 1 || len(fl.started) != before+1 {
		t.Errorf("reopened, watches %v and follows %v; want the log watched and the run followed again", f.watching, fl.started)
	}
}

// TestEscClosesFromEveryStep checks that esc closes the modal at once
// from any pane, not a pane at a time.
func TestEscClosesFromEveryStep(t *testing.T) {
	for _, tab := range []int{0, 1, 2} {
		m, h := newModal(t, newFake(), wideW, wideH)
		for range tab {
			h.keys("tab")
		}
		h.keys("esc")
		if got := h.take(); len(got) != 1 || got[0] != (ui.LeaveModalMsg{Modal: m}) {
			t.Errorf("esc on pane %d sent %v, want the modal closed", tab, got)
		}
	}
}

// TestEscClearsWhatIsShownFirst checks that esc closes the log at once over
// its search, while it takes away the find of the runs before it closes the
// modal, from the keys and as the app's dismiss intent.
func TestEscClearsWhatIsShownFirst(t *testing.T) {
	t.Run("log search", func(t *testing.T) {
		_, h := newModal(t, newFake(), wideW, wideH)
		l := h.openLog(t)
		h.keys("/", "s", "t", "enter")
		if l.log.Query() == "" {
			t.Fatal("the log shows no search")
		}
		h.keys("esc")
		if h.log != nil {
			t.Errorf("esc left the log open over its search")
		}
	})
	t.Run("runs find", func(t *testing.T) {
		// The find is unbound by default.
		keys := testKeys()
		keys.Set("actions_runs.find", []string{"/"})
		m, h := newModalKeys(t, keys, newFake(), wideW, wideH)
		h.keys("/", "C", "I", "enter")
		if m.runs.FindQuery() == "" {
			t.Fatal("the runs show no find")
		}
		cmd, ok := m.Act(ui.ActDismiss)
		h.run(cmd)
		if !ok || m.runs.FindQuery() != "" || len(h.take()) != 0 {
			t.Fatalf("dismiss: taken %v, find %q, want it cleared and the modal open", ok, m.runs.FindQuery())
		}
		cmd, _ = m.Act(ui.ActDismiss)
		h.run(cmd)
		if got := h.take(); len(got) != 1 || got[0] != (ui.LeaveModalMsg{Modal: m}) {
			t.Errorf("the second dismiss sent %v, want the modal closed", got)
		}
	})
}

// TestBackspaceTypesInThePrompts checks that the prompts of the runs and of
// the log keep backspace to delete: it neither steps back nor closes.
func TestBackspaceTypesInThePrompts(t *testing.T) {
	keys := testKeys()
	keys.Set("actions_runs.find", []string{"/"})
	m, h := newModalKeys(t, keys, newFake(), wideW, wideH)
	h.keys("/", "a", "b", "backspace")
	if !m.runs.Capturing() || m.focus != runsPane || len(h.take()) != 0 {
		t.Fatal("backspace left the prompt of the runs")
	}
	h.keys("esc")
	l := h.openLog(t)
	h.keys("/", "a", "b", "backspace")
	if h.log == nil || !l.log.Capturing() || len(h.take()) != 0 {
		t.Error("backspace left the search of the log")
	}
}

// TestActBackStepsBack checks the app's back intent: it steps the log back
// to the jobs and those to the runs, which leave it to the app.
func TestActBackStepsBack(t *testing.T) {
	m, h := newModal(t, newFake(), wideW, wideH)
	h.keys("tab")
	for _, want := range []pane{runsPane} {
		cmd, ok := m.Act(ui.ActBack)
		if !ok || cmd != nil || m.focus != want {
			t.Fatalf("Act(back) = %v, %v on pane %d, want it taken on %d", cmd, ok, m.focus, want)
		}
	}
	if cmd, ok := m.Act(ui.ActBack); ok || cmd != nil || m.focus != runsPane {
		t.Errorf("Act(back) on the runs = %v, %v, want it left to the app", cmd, ok)
	}
}

// TestEscClosesTheLogOverItsSearch checks that esc closes the log on the
// first press with a search shown, and that the app's dismiss intent is left
// to the log, which the key closes.
func TestEscClosesTheLogOverItsSearch(t *testing.T) {
	_, h := newModal(t, newFake(), wideW, wideH)
	h.openLog(t)
	h.keys("/", "s", "t", "enter")
	h.keys("esc")
	if h.log != nil {
		t.Error("esc left the log open with a search shown")
	}
}

// TestEscIsLabelledForTheFindAndTheFilterOfTheRuns checks that the help
// names what esc clears on the runs: the find first, then the quick filter.
func TestEscIsLabelledForTheFindAndTheFilterOfTheRuns(t *testing.T) {
	keys := testKeys()
	keys.Set("actions_runs.find", []string{"/"})
	keys.Set("actions_runs.quick_filter", []string{"&"})
	m, h := newModalKeys(t, keys, newFake(), wideW, wideH)
	desc := func() string {
		b, _, _ := uitest.Winner(m.KeyLayers(), "esc")
		return b.Help().Desc
	}
	if got := desc(); got != "close" {
		t.Fatalf("esc reads %q, want close", got)
	}
	h.keys("&", "C", "I", "enter")
	if got := desc(); got != "clear filter" {
		t.Fatalf("esc reads %q with a quick filter shown, want clear filter", got)
	}
	h.keys("/", "C", "I", "enter")
	if got := desc(); got != "clear find" {
		t.Errorf("esc reads %q with a find shown, want clear find", got)
	}
}
