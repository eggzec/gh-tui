package actions

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

var (
	repo    = core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	testNow = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
)

func at(d time.Duration) time.Time { return testNow.Add(-d) }

// IDs of the runs and jobs of the fake.
const (
	failedRun    = 4812
	passedRun    = 4811
	runningRun   = 4810
	cancelledRun = 4809

	lintJob    = 100
	ubuntuJob  = 101
	macosJob   = 102
	buildJob   = 103
	runLintJob = 200
)

func testRuns() []core.Run {
	return []core.Run{
		{
			ID: failedRun, Attempt: 1, Name: "CI", DisplayTitle: "fix: skip capability queries when input is disabled",
			Number: 4812, Event: "push", Branch: "main", HeadSHA: "f00dcafe", Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
			Actor: "drew", WorkflowID: 1, CreatedAt: at(2 * time.Hour), RunStartedAt: at(2 * time.Hour),
			UpdatedAt: at(2*time.Hour - 3*time.Minute - 12*time.Second), URL: "https://github.com/charmbracelet/bubbletea/actions/runs/4812",
		},
		{
			ID: runningRun, Attempt: 1, Name: "lint", DisplayTitle: "feat: clipboard without OSC52",
			Number: 4810, Event: "pull_request", Branch: "feat/osc52", Status: core.RunInProgress,
			Actor: "aymanbagabas", WorkflowID: 2, CreatedAt: at(90 * time.Second), RunStartedAt: at(90 * time.Second),
			UpdatedAt: at(10 * time.Second), URL: "https://github.com/charmbracelet/bubbletea/actions/runs/4810",
		},
		{
			ID: passedRun, Attempt: 2, Name: "CI", DisplayTitle: "docs: add authors",
			Number: 4811, Event: "push", Branch: "main", Status: core.RunCompleted, Conclusion: core.ConclusionSuccess,
			Actor: "meowgorithm", WorkflowID: 1, CreatedAt: at(3 * time.Hour), RunStartedAt: at(3 * time.Hour),
			UpdatedAt: at(3*time.Hour - 95*time.Second), URL: "https://github.com/charmbracelet/bubbletea/actions/runs/4811",
		},
		{
			ID: cancelledRun, Attempt: 1, Name: "CI", DisplayTitle: "chore: bump deps",
			Number: 4809, Event: "schedule", Branch: "main", Status: core.RunCompleted, Conclusion: core.ConclusionCancelled,
			Actor: "drew", WorkflowID: 1, CreatedAt: at(6 * time.Hour), RunStartedAt: at(6 * time.Hour),
			UpdatedAt: at(6*time.Hour - 20*time.Second),
		},
	}
}

func step(n int, name string, c core.Conclusion, start, took time.Duration) core.Step {
	s := core.Step{Number: n, Name: name, Status: core.RunCompleted, Conclusion: c, StartedAt: at(start)}
	s.CompletedAt = s.StartedAt.Add(took)
	return s
}

func job(id, runID int64, name string, c core.Conclusion, start, took time.Duration, steps ...core.Step) core.Job {
	j := core.Job{
		ID: id, RunID: runID, Attempt: 1, Name: name, Status: core.RunCompleted, Conclusion: c, StartedAt: at(start),
		Steps: steps, URL: fmt.Sprintf("https://github.com/charmbracelet/bubbletea/actions/runs/%d/job/%d", runID, id),
	}
	j.CompletedAt = j.StartedAt.Add(took)
	return j
}

// attempt returns j of attempt n.
func attempt(j core.Job, n int) core.Job {
	j.Attempt = n
	return j
}

func testJobs() map[int64][]core.Job {
	h := 2 * time.Hour
	running := core.Job{
		ID: runLintJob, RunID: runningRun, Attempt: 1, Name: "lint", Status: core.RunInProgress, StartedAt: at(80 * time.Second),
		Steps: []core.Step{
			step(1, "Set up job", core.ConclusionSuccess, 80*time.Second, 2*time.Second),
			{Number: 2, Name: "Run golangci-lint", Status: core.RunInProgress, StartedAt: at(78 * time.Second)},
			{Number: 3, Name: "Post Run actions/checkout@v4", Status: core.RunQueued},
		},
	}
	return map[int64][]core.Job{
		failedRun: {
			job(lintJob, failedRun, "lint", core.ConclusionSuccess, h, 41*time.Second),
			job(ubuntuJob, failedRun, "test (ubuntu-latest, 1.26)", core.ConclusionFailure, h, 3*time.Minute+2*time.Second,
				step(1, "Set up job", core.ConclusionSuccess, h, 2*time.Second),
				step(2, "Run go test -race ./...", core.ConclusionFailure, h-2*time.Second, 2*time.Minute+48*time.Second),
				step(3, "Complete job", core.ConclusionSuccess, h-2*time.Second-2*time.Minute-48*time.Second, time.Second),
			),
			job(macosJob, failedRun, "test (macos-latest, 1.26)", core.ConclusionSuccess, h, 2*time.Minute+55*time.Second),
			job(buildJob, failedRun, "build", core.ConclusionSkipped, h, 0),
		},
		passedRun: {
			attempt(job(300, passedRun, "test", core.ConclusionSuccess, 3*time.Hour, 90*time.Second), 2),
		},
		runningRun: {running},
		cancelledRun: {
			job(400, cancelledRun, "test", core.ConclusionCancelled, 6*time.Hour, 20*time.Second),
		},
	}
}

func testLog() core.Log {
	h := 2 * time.Hour
	line := func(d time.Duration, text string, kind core.LogKind, st int) core.LogLine {
		return core.LogLine{Time: at(d), Text: text, Kind: kind, Step: st}
	}
	return core.Log{Lines: []core.LogLine{
		line(h, "Current runner version: '2.337.0'", core.LogPlain, 1),
		line(h, "Operating System", core.LogGroup, 1),
		line(h, "Ubuntu 24.04", core.LogPlain, 1),
		line(h, "", core.LogEndGroup, 1),
		line(h-2*time.Second, "Run go test -race ./...", core.LogGroup, 2),
		line(h-2*time.Second, "go test -race ./...", core.LogPlain, 2),
		line(h-2*time.Second, "", core.LogEndGroup, 2),
		line(h-10*time.Second, "ok  \tcharm.land/bubbletea/v2/internal\t1.2s", core.LogPlain, 2),
		line(h-2*time.Minute, "--- FAIL: TestProgram (0.31s)", core.LogPlain, 2),
		line(h-2*time.Minute, "    teatest_test.go:54: want a frame, got none", core.LogPlain, 2),
		line(h-2*time.Minute, "FAIL\tcharm.land/bubbletea/v2\t2.1s", core.LogPlain, 2),
		line(h-2*time.Minute-40*time.Second, "Process completed with exit code 1.", core.LogError, 2),
		line(h-2*time.Minute-50*time.Second, "Cleaning up orphan processes", core.LogPlain, 3),
	}}
}

// fake serves runs, jobs and logs from memory, and counts its reads. Its
// changes work as the service's do: they show at once and roll back when
// refuse is set.
type fake struct {
	mu        sync.Mutex
	runs      []core.Run
	jobs      map[int64][]core.Job
	logs      map[int64]core.Log
	workflows []core.Workflow
	// keptWorkflows serves the workflows Stale, as an earlier session kept
	// them, until a read with Again set.
	keptWorkflows bool
	// cachedJobs and cachedLogs hold what the Cached reads find.
	cachedJobs map[int64]bool
	cachedLogs map[int64]bool
	// moreJobs makes the jobs tell that there are more than the service
	// reads.
	moreJobs bool
	// notes are the annotations of jobs, and cachedNotes those in memory.
	notes       map[int64][]core.Annotation
	cachedNotes map[int64]bool

	runsErr, jobsErr, logErr error
	// logErrs fails the logs of some jobs.
	logErrs map[int64]error
	// refuse fails the changes, as GitHub refusing them.
	refuse error

	queries   []actionssvc.RunsQuery
	jobReads  []int64
	logReads  []int64
	noteReads []int64
	wfReads   int
	wfAgain   []bool
	runReads  int
	sent      []string
}

func newFake() *fake {
	return &fake{
		runs:        testRuns(),
		jobs:        testJobs(),
		logs:        map[int64]core.Log{ubuntuJob: testLog()},
		workflows:   []core.Workflow{{ID: 1, Name: "CI"}, {ID: 2, Name: "lint"}, {ID: 3, Name: "Build and test"}},
		cachedJobs:  map[int64]bool{},
		cachedLogs:  map[int64]bool{},
		logErrs:     map[int64]error{},
		notes:       map[int64][]core.Annotation{ubuntuJob: testNotes()},
		cachedNotes: map[int64]bool{},
	}
}

func (f *fake) Runs(_ context.Context, q actionssvc.RunsQuery) (core.Page[core.Run], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, q)
	if f.runsErr != nil {
		return core.Page[core.Run]{}, f.runsErr
	}
	var out []core.Run
	for i := range f.runs {
		r := &f.runs[i]
		st := q.Filter.Status
		switch {
		case st != "" && st != string(r.Status) && st != string(r.Conclusion):
		case q.Filter.Actor != "" && q.Filter.Actor != r.Actor:
		case q.Filter.Branch != "" && q.Filter.Branch != r.Branch:
		case q.Filter.Event != "" && q.Filter.Event != r.Event:
		case q.Filter.WorkflowID != 0 && q.Filter.WorkflowID != r.WorkflowID:
		default:
			out = append(out, *r)
		}
	}
	return core.Page[core.Run]{Items: out}, nil
}

func (f *fake) CachedRun(_ core.RepoRef, runID int64) (core.Run, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.runs, func(r core.Run) bool { return r.ID == runID })
	if i < 0 {
		return core.Run{}, false
	}
	return f.runs[i], true
}

func (f *fake) Run(_ context.Context, r core.RepoRef, runID int64) (core.Run, error) {
	f.mu.Lock()
	f.runReads++
	f.mu.Unlock()
	run, _ := f.CachedRun(r, runID)
	return run, nil
}

func (f *fake) Workflows(_ context.Context, q actionssvc.WorkflowsQuery) (core.Page[core.Workflow], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wfReads++
	f.wfAgain = append(f.wfAgain, q.Again)
	if q.Again {
		f.keptWorkflows = false
	}
	return core.Page[core.Workflow]{Items: f.workflows, Stale: f.keptWorkflows}, nil
}

func (f *fake) CachedAllJobs(q actionssvc.JobsQuery) (core.Page[core.Job], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cachedJobs[q.RunID] {
		return core.Page[core.Job]{}, false
	}
	return f.page(q), true
}

func (f *fake) page(q actionssvc.JobsQuery) core.Page[core.Job] {
	var out []core.Job
	jobs := f.jobs[q.RunID]
	for i := range jobs {
		if jobs[i].Attempt == q.Attempt || q.Attempt == 0 {
			out = append(out, jobs[i])
		}
	}
	p := core.Page[core.Job]{Items: out}
	if f.moreJobs {
		p.Next = "more"
	}
	return p
}

func (f *fake) AllJobs(_ context.Context, q actionssvc.JobsQuery) (core.Page[core.Job], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobReads = append(f.jobReads, q.RunID)
	if f.jobsErr != nil {
		return core.Page[core.Job]{}, f.jobsErr
	}
	f.cachedJobs[q.RunID] = true
	return f.page(q), nil
}

func (f *fake) CachedLog(_ core.RepoRef, jobID int64) (core.Log, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cachedLogs[jobID] {
		return core.Log{}, false
	}
	return f.logs[jobID], true
}

func (f *fake) Log(_ context.Context, _ core.RepoRef, jobID int64) (core.Log, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logReads = append(f.logReads, jobID)
	if err := f.logErrs[jobID]; err != nil {
		return core.Log{}, err
	}
	if f.logErr != nil {
		return core.Log{}, f.logErr
	}
	f.cachedLogs[jobID] = true
	return f.logs[jobID], nil
}

// change shows run runID, and its jobs that match, with status at once,
// and returns the Op that sends what, as the service does.
func (f *fake) change(what string, runID int64, status core.RunStatus, match func(core.Job) bool) *optimistic.Op {
	f.mu.Lock()
	defer f.mu.Unlock()
	runs, jobs := slices.Clone(f.runs), slices.Clone(f.jobs[runID])
	for i := range f.runs {
		if f.runs[i].ID == runID {
			f.runs[i].Status, f.runs[i].Conclusion = status, core.ConclusionNone
		}
	}
	edited := slices.Clone(jobs)
	for i := range edited {
		if match(edited[i]) {
			edited[i].Status, edited[i].Conclusion = status, core.ConclusionNone
		}
	}
	f.jobs[runID] = edited
	return optimistic.New(func(context.Context) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.sent = append(f.sent, what)
		return f.refuse
	}, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.runs, f.jobs[runID] = runs, jobs
	})
}

func (f *fake) RerunRun(_ core.RepoRef, runID int64) *optimistic.Op {
	return f.change("rerun", runID, core.RunQueued, func(core.Job) bool { return true })
}

func (f *fake) RerunFailedJobs(_ core.RepoRef, runID int64) *optimistic.Op {
	return f.change("rerun failed", runID, core.RunQueued, func(j core.Job) bool {
		return j.Conclusion.Failed() || j.Conclusion == core.ConclusionCancelled
	})
}

func (f *fake) RerunJob(_ core.RepoRef, runID, jobID int64) *optimistic.Op {
	return f.change(fmt.Sprintf("rerun job %d", jobID), runID, core.RunQueued, func(j core.Job) bool { return j.ID == jobID })
}

func (f *fake) CancelRun(_ core.RepoRef, runID int64) *optimistic.Op {
	return f.change("cancel", runID, core.RunCancelling, func(j core.Job) bool { return !j.Done() })
}

// setRun replaces run r, as a poll would cache it.
func (f *fake) setRun(r core.Run) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.runs {
		if f.runs[i].ID == r.ID {
			f.runs[i] = r
		}
	}
}

// setJobs replaces the jobs of run runID, as a poll would cache them.
func (f *fake) setJobs(runID int64, jobs []core.Job) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobs[runID] = jobs
	f.cachedJobs[runID] = true
}

func (f *fake) counts() (runs, jobs, logs int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.queries), len(f.jobReads), len(f.logReads)
}

// follows records what the modal follows.
type follows struct {
	mu      sync.Mutex
	started []int64
	stopped []int64
}

func (fl *follows) follow(_ core.RepoRef, runID int64) func() {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	fl.started = append(fl.started, runID)
	return func() {
		fl.mu.Lock()
		defer fl.mu.Unlock()
		fl.stopped = append(fl.stopped, runID)
	}
}

// viewer is the user of the tests.
func viewer(context.Context) (string, error) { return "drew", nil }

// notes are the annotations of the failed job of the fake.
func testNotes() []core.Annotation {
	return []core.Annotation{
		{Path: "tea_test.go", StartLine: 54, EndLine: 54, Level: core.AnnotationFailure, Message: "want a frame, got none"},
		{Path: ".github", Level: core.AnnotationFailure, Message: "Process completed with exit code 1."},
	}
}

func (f *fake) CachedAnnotations(q actionssvc.AnnotationsQuery) (core.Page[core.Annotation], bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cachedNotes[q.CheckRunID] {
		return core.Page[core.Annotation]{}, false
	}
	return core.Page[core.Annotation]{Items: f.notes[q.CheckRunID]}, true
}

func (f *fake) Annotations(_ context.Context, q actionssvc.AnnotationsQuery) (core.Page[core.Annotation], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.noteReads = append(f.noteReads, q.CheckRunID)
	f.cachedNotes[q.CheckRunID] = true
	return core.Page[core.Annotation]{Items: f.notes[q.CheckRunID]}, nil
}
