package checks

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

// The sizes inside the frame on terminals of 190 by 50 and 80 by 24.
const (
	wideW, wideH     = 148, 38
	narrowW, narrowH = 60, 18
)

var (
	repo    = core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	testNow = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	query   = actionssvc.ChecksQuery{Repo: repo, Number: 1500}
)

func at(d time.Duration) time.Time { return testNow.Add(-d) }

// IDs of the runs and jobs of the fake.
const (
	ciRun    = 4812
	buildRun = 4813
	lintJob  = 100
	testJob  = 101
	buildJob = 103
	codecov  = 900
)

func testChecks() core.Checks {
	done := func(id int64, name string, c core.Conclusion, took time.Duration) core.Check {
		return core.Check{
			ID: id, Name: name, Status: core.RunCompleted, Conclusion: c, JobID: id, RunID: ciRun, Workflow: "CI",
			StartedAt: at(time.Hour), CompletedAt: at(time.Hour - took),
			DetailsURL: "https://github.com/charmbracelet/bubbletea/actions/runs/4812/job/" + strconv.FormatInt(id, 10),
		}
	}
	test := done(testJob, "test (ubuntu-latest)", core.ConclusionFailure, 3*time.Minute+2*time.Second)
	test.Required, test.Annotations = true, 2
	return core.Checks{
		SHA: "f00dcafe", State: core.ChecksFailure, Total: 6,
		Runs: []core.Check{
			done(lintJob, "lint", core.ConclusionSuccess, 41*time.Second),
			test,
			{
				ID: buildJob, Name: "build", Status: core.RunInProgress, JobID: buildJob, RunID: buildRun, Workflow: "Build",
				StartedAt: at(90 * time.Second),
			},
			{
				ID: codecov, Name: "codecov/patch", Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
				StartedAt: at(50 * time.Minute), CompletedAt: at(49 * time.Minute), DetailsURL: "https://codecov.io/gh/charmbracelet/bubbletea/pull/1500",
				Title: "62.50% of diff hit (target 80.00%)", Summary: "**3** lines in `tea.go` are not covered.", Text: "| File | Coverage |\n|---|---|\n| tea.go | 62.5% |",
			},
		},
		Statuses: []core.StatusContext{
			{Context: "ci/circleci", State: "success", Description: "Your tests passed", TargetURL: "https://circleci.com/gh/x/1"},
			{Context: "netlify/deploy", State: "pending", Description: "Deploying the preview", Required: true},
		},
	}
}

func testRuns() map[int64]core.Run {
	return map[int64]core.Run{
		ciRun: {
			ID: ciRun, Attempt: 1, Name: "CI", Number: 4812, Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
			HeadSHA: "f00dcafe", CreatedAt: at(time.Hour), UpdatedAt: at(time.Hour - 3*time.Minute),
		},
		buildRun: {
			ID: buildRun, Attempt: 1, Name: "Build", Number: 4813, Status: core.RunInProgress,
			HeadSHA: "f00dcafe", CreatedAt: at(90 * time.Second), UpdatedAt: at(10 * time.Second),
		},
	}
}

func testJobs() map[int64][]core.Job {
	step := func(n int, name string, c core.Conclusion) core.Step {
		return core.Step{Number: n, Name: name, Status: core.RunCompleted, Conclusion: c, StartedAt: at(time.Hour), CompletedAt: at(time.Hour - time.Minute)}
	}
	return map[int64][]core.Job{
		ciRun: {
			{ID: lintJob, RunID: ciRun, Attempt: 1, Name: "lint", Status: core.RunCompleted, Conclusion: core.ConclusionSuccess, StartedAt: at(time.Hour), CompletedAt: at(time.Hour - 41*time.Second)},
			{
				ID: testJob, RunID: ciRun, Attempt: 1, Name: "test (ubuntu-latest)", Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
				StartedAt: at(time.Hour), CompletedAt: at(time.Hour - 3*time.Minute), URL: "https://github.com/charmbracelet/bubbletea/actions/runs/4812/job/101",
				Steps: []core.Step{step(1, "Set up job", core.ConclusionSuccess), step(2, "Run go test ./...", core.ConclusionFailure)},
			},
		},
		buildRun: {
			{
				ID: buildJob, RunID: buildRun, Attempt: 1, Name: "build", Status: core.RunInProgress, StartedAt: at(90 * time.Second),
				Steps: []core.Step{step(1, "Set up job", core.ConclusionSuccess), {Number: 2, Name: "go build ./...", Status: core.RunInProgress, StartedAt: at(80 * time.Second)}},
			},
		},
	}
}

func testLog() core.Log {
	line := func(text string, kind core.LogKind, st int) core.LogLine {
		return core.LogLine{Time: at(time.Hour), Text: text, Kind: kind, Step: st}
	}
	return core.Log{Lines: []core.LogLine{
		line("Current runner version: '2.337.0'", core.LogPlain, 1),
		line("go test ./...", core.LogPlain, 2),
		line("--- FAIL: TestProgram (0.31s)", core.LogPlain, 2),
		line("    tea_test.go:54: want a frame, got none", core.LogPlain, 2),
		line("Process completed with exit code 1.", core.LogError, 2),
	}}
}

// fake serves the checks, runs, jobs, logs and annotations from memory,
// and counts its reads.
type fake struct {
	mu     sync.Mutex
	checks core.Checks
	// cached is set once the checks were read, which CachedChecks then
	// serves.
	cached    bool
	checksErr error
	runs      map[int64]core.Run
	jobs      map[int64][]core.Job
	logs      map[int64]core.Log
	notes     map[int64][]core.Annotation

	checkReads, runReads, jobReads, noteReads int
	invalidated                               int
	sent                                      []string
}

func newFake() *fake {
	return &fake{
		checks: testChecks(), runs: testRuns(), jobs: testJobs(),
		logs: map[int64]core.Log{testJob: testLog()},
		notes: map[int64][]core.Annotation{testJob: {
			{Path: "tea_test.go", StartLine: 54, Level: core.AnnotationFailure, Message: "want a frame, got none"},
			{Path: ".github", Level: core.AnnotationFailure, Message: "Process completed with exit code 1."},
		}},
	}
}

func (f *fake) CachedChecks(q actionssvc.ChecksQuery) (core.Checks, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checks, f.cached && q == query
}

func (f *fake) Checks(context.Context, actionssvc.ChecksQuery) (core.Checks, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkReads++
	if f.checksErr != nil {
		return core.Checks{}, f.checksErr
	}
	f.cached = true
	return f.checks, nil
}

func (f *fake) CachedRun(_ core.RepoRef, runID int64) (core.Run, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.runs[runID]
	return r, ok && f.runReads > 0
}

func (f *fake) Run(_ context.Context, _ core.RepoRef, runID int64) (core.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runReads++
	return f.runs[runID], nil
}

func (f *fake) CachedAllJobs(q actionssvc.JobsQuery) (core.Page[core.Job], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.allJobs(q), f.jobReads > 0
}

func (f *fake) AllJobs(_ context.Context, q actionssvc.JobsQuery) (core.Page[core.Job], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobReads++
	return f.allJobs(q), nil
}

// allJobs is every page of the jobs of run q.RunID.
func (f *fake) allJobs(q actionssvc.JobsQuery) core.Page[core.Job] {
	return core.Page[core.Job]{Items: f.jobs[q.RunID]}
}

func (f *fake) CachedLog(core.RepoRef, int64) (core.Log, bool) { return core.Log{}, false }

func (f *fake) Log(_ context.Context, _ core.RepoRef, jobID int64) (core.Log, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logs[jobID], nil
}

func (f *fake) CachedAnnotations(actionssvc.AnnotationsQuery) (core.Page[core.Annotation], bool) {
	return core.Page[core.Annotation]{}, false
}

func (f *fake) Annotations(_ context.Context, q actionssvc.AnnotationsQuery) (core.Page[core.Annotation], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.noteReads++
	return core.Page[core.Annotation]{Items: f.notes[q.CheckRunID]}, nil
}

func (f *fake) Invalidate(core.RepoRef) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidated++
}

// RerunFailedJobs shows the run queued and its failed jobs queued, as the
// service does, and rolls back if refused.
func (f *fake) RerunFailedJobs(_ core.RepoRef, runID int64) *optimistic.Op {
	f.mu.Lock()
	defer f.mu.Unlock()
	run, jobs := f.runs[runID], append([]core.Job(nil), f.jobs[runID]...)
	r := run
	r.Status, r.Conclusion = core.RunQueued, core.ConclusionNone
	f.runs[runID] = r
	edited := append([]core.Job(nil), jobs...)
	for i := range edited {
		if edited[i].Conclusion.Failed() {
			edited[i].Status, edited[i].Conclusion = core.RunQueued, core.ConclusionNone
		}
	}
	f.jobs[runID] = edited
	return optimistic.New(func(context.Context) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.sent = append(f.sent, "rerun failed")
		return nil
	}, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.runs[runID], f.jobs[runID] = run, jobs
	})
}

func (f *fake) setChecks(c core.Checks) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks, f.cached = c, true
}

func testTheme() ui.Theme {
	p, err := config.Default().Palette(true)
	if err != nil {
		panic(err)
	}
	return ui.NewTheme(p, true)
}

// retModal is the modal the step is shown in, for the returns of previews.
type retModal struct{ ui.Modal }

var ret = &retModal{}

// forTests stops the timers, so that tests don't wait.
func forTests() Option {
	return func(o *options) { o.tick = 0 }
}

// newStep returns the step of the fake's pull request, of width by height,
// loaded.
func newStep(tb testing.TB, f Service, width, height int, opts ...Option) (*Step, *host) {
	tb.Helper()
	opts = append([]Option{
		forTests(), WithClock(func() time.Time { return testNow }), WithIcons(ui.NewIcons(config.IconsUnicode)), WithReturn(ret),
	}, opts...)
	s := New(tb.Context(), f, repo, query.Number, config.Default().Keys, opts...)
	s.SetTheme(testTheme())
	s.SetSize(width, height)
	h := &host{s: s}
	h.run(s.Init())
	return s, h
}

// host runs the commands of a step the way the modal and the app would,
// feeding every message back to it, and keeps those meant for them.
type host struct {
	s   *Step
	got []tea.Msg
}

func (h *host) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case nil, spinner.TickMsg:
		return
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
		return
	case ui.OpenMsg, ui.NotifyMsg, ui.OpenFileMsg, CloseMsg:
		h.got = append(h.got, msg)
		return
	case ui.DoneMsg:
		h.got = append(h.got, msg)
	}
	if cmds, ok := sequence(msg); ok {
		for _, c := range cmds {
			h.run(c)
		}
		return
	}
	h.run(h.s.Update(msg))
}

func (h *host) send(msg tea.Msg) { h.run(func() tea.Msg { return msg }) }

func (h *host) keys(ks ...string) {
	for _, k := range ks {
		h.run(h.s.Update(press(k)))
	}
}

func (h *host) take() []tea.Msg {
	out := h.got
	h.got = nil
	return out
}

func sequence(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}
	cmds := make([]tea.Cmd, v.Len())
	for i := range cmds {
		cmds[i], _ = reflect.TypeAssert[tea.Cmd](v.Index(i))
	}
	return cmds, true
}

func press(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "ctrl+r":
		return tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// text is the view without styles, its spaces collapsed.
func text(s *Step) string {
	return strings.Join(strings.Fields(ansi.Strip(s.View())), " ")
}

func assertFits(tb testing.TB, v string, width, height int) {
	tb.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		tb.Errorf("view has %d lines, want %d", len(lines), height)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			tb.Errorf("line %d is %d wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}

// watches records what the step polls: the checks, and the runs it
// follows.
type watches struct {
	mu                   sync.Mutex
	started, stopped     []string
	followed, unfollowed []int64
}

func (w *watches) watch(q actionssvc.ChecksQuery) func() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.started = append(w.started, actionssvc.ChecksSyncKey(q))
	return func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.stopped = append(w.stopped, actionssvc.ChecksSyncKey(q))
	}
}

func (w *watches) follow(_ core.RepoRef, runID int64) func() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.followed = append(w.followed, runID)
	return func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.unfollowed = append(w.unfollowed, runID)
	}
}

var errBoom = errors.New("boom")
