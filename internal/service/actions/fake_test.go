package actions

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var _ API = (*github.Client)(nil)

var repo = core.RepoRef{Owner: "octo-org", Name: "hello"}

// errDial is how a request fails while GitHub can't be reached.
var errDial = fmt.Errorf("%w: %w", core.ErrOffline, &url.Error{Op: "Get", URL: "https://api.github.com/", Err: errors.New("connection refused")})

// errNotFound is how GitHub refuses what the account can't see.
var errNotFound = fmt.Errorf("github: 404 Not Found: %w", core.ErrNotFound)

// at is when the runs of the fake started.
var at = time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)

// fakeGitHub is a repository with some workflow runs, each with its jobs.
// It answers conditional requests with a 304 while nothing changed, and
// fails every request with err while it is set, or those of a method with
// errs of its name. It records the calls.
type fakeGitHub struct {
	mu sync.Mutex
	// runs is newest first.
	runs      []core.Run
	jobs      map[int64][]core.Job
	logs      map[int64]string
	truncated bool
	checks    map[string]core.Checks
	notes     map[int64][]core.Annotation
	workflows []core.Workflow
	err       error
	errs      map[string]error
	calls     []string
}

func newFake() *fakeGitHub {
	f := &fakeGitHub{
		jobs:      map[int64][]core.Job{},
		logs:      map[int64]string{},
		checks:    map[string]core.Checks{},
		notes:     map[int64][]core.Annotation{},
		errs:      map[string]error{},
		workflows: []core.Workflow{{ID: 7, Name: "build", Path: ".github/workflows/build.yml", State: "active"}},
	}
	// Run 1 completed with a failed job, run 2 is in progress.
	f.runs = []core.Run{
		{ID: 2, Attempt: 1, Name: "build", Number: 2, Status: core.RunInProgress, CreatedAt: at.Add(time.Hour)},
		{ID: 1, Attempt: 1, Name: "build", Number: 1, Status: core.RunCompleted, Conclusion: core.ConclusionFailure, CreatedAt: at},
	}
	f.jobs[1] = []core.Job{
		doneJob(11, 1, core.ConclusionSuccess),
		doneJob(12, 1, core.ConclusionFailure),
	}
	f.jobs[2] = []core.Job{
		doneJob(21, 2, core.ConclusionSuccess),
		{ID: 22, RunID: 2, Attempt: 1, Name: "test", Status: core.RunInProgress, StartedAt: at.Add(time.Hour), Steps: []core.Step{
			{Number: 1, Name: "Set up job", Status: core.RunCompleted, Conclusion: core.ConclusionSuccess, StartedAt: at.Add(time.Hour), CompletedAt: at.Add(time.Hour + time.Second)},
			{Number: 2, Name: "Run go test", Status: core.RunInProgress, StartedAt: at.Add(time.Hour + time.Second)},
		}},
	}
	f.logs[11] = "2026-09-22T10:00:00.1Z setting up\n2026-09-22T10:00:01.5Z ##[group]Run go test\n2026-09-22T10:00:02Z ok\n"
	f.logs[12] = "2026-09-22T10:00:00.1Z setting up\n2026-09-22T10:00:01.5Z ##[group]Run go test\n2026-09-22T10:00:02Z ##[error]FAIL\n"
	return f
}

// doneJob is job id of run, which completed with c, in two steps.
func doneJob(id, run int64, c core.Conclusion) core.Job {
	return core.Job{
		ID: id, RunID: run, Attempt: 1, Name: "job " + strconv.FormatInt(id, 10), Status: core.RunCompleted, Conclusion: c,
		StartedAt: at, CompletedAt: at.Add(2 * time.Second),
		Steps: []core.Step{
			{Number: 1, Name: "Set up job", Status: core.RunCompleted, Conclusion: core.ConclusionSuccess, StartedAt: at, CompletedAt: at.Add(time.Second)},
			{Number: 2, Name: "Run go test", Status: core.RunCompleted, Conclusion: c, StartedAt: at.Add(time.Second), CompletedAt: at.Add(2 * time.Second)},
		},
	}
}

// change applies fn to the fake under its lock.
func (f *fakeGitHub) change(fn func(f *fakeGitHub)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeGitHub) fail(err error) {
	f.change(func(f *fakeGitHub) { f.err = err })
}

// failCall makes calls of method fail with err, or succeed again for nil.
func (f *fakeGitHub) failCall(method string, err error) {
	f.change(func(f *fakeGitHub) { f.errs[method] = err })
}

// record notes a call of method. f.mu must be held.
func (f *fakeGitHub) record(method, format string, args ...any) error {
	f.calls = append(f.calls, method+" "+fmt.Sprintf(format, args...))
	if err := f.errs[method]; err != nil {
		return err
	}
	return f.err
}

// take returns the calls since the last take.
func (f *fakeGitHub) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := f.calls
	f.calls = nil
	return calls
}

// etag is the ETag of v, which changes with v.
func etag(v any) string {
	return fmt.Sprintf(`"%x"`, sha256.Sum256(fmt.Appendf(nil, "%+v", v)))
}

// answer returns v, or a 304 if cond is its ETag.
func answer[V any](v V, cond github.Conditional) (V, github.Response) {
	tag := etag(v)
	if cond.ETag == tag {
		var zero V
		return zero, github.Response{StatusCode: 304, NotModified: true, ETag: tag}
	}
	return v, github.Response{StatusCode: 200, ETag: tag}
}

func condMark(cond github.Conditional) string {
	if cond.ETag != "" {
		return " if-none-match"
	}
	return ""
}

func (f *fakeGitHub) ListRuns(_ context.Context, r core.RepoRef, filter core.RunFilter, cursor string, perPage int, cond github.Conditional) (core.Page[core.Run], github.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("ListRuns", "%s status=%s cursor=%s per_page=%d%s", r, filter.Status, cursor, perPage, condMark(cond)); err != nil {
		return core.Page[core.Run]{}, github.Response{}, err
	}
	var items []core.Run
	for i := range f.runs {
		run := &f.runs[i]
		if filter.Status == "" || string(run.Status) == filter.Status || string(run.Conclusion) == filter.Status {
			items = append(items, *run)
		}
	}
	start, _ := strconv.Atoi(cursor)
	start = min(start, len(items))
	end := min(start+perPage, len(items))
	page := core.Page[core.Run]{Items: slices.Clone(items[start:end])}
	if end < len(items) {
		page.Next = strconv.Itoa(end)
	}
	v, res := answer(page, cond)
	return v, res, nil
}

func (f *fakeGitHub) GetRun(_ context.Context, r core.RepoRef, runID int64, cond github.Conditional) (core.Run, github.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("GetRun", "%s %d%s", r, runID, condMark(cond)); err != nil {
		return core.Run{}, github.Response{}, err
	}
	i := slices.IndexFunc(f.runs, func(run core.Run) bool { return run.ID == runID })
	if i < 0 {
		return core.Run{}, github.Response{}, errNotFound
	}
	v, res := answer(f.runs[i], cond)
	return v, res, nil
}

func (f *fakeGitHub) ListWorkflows(_ context.Context, r core.RepoRef, cursor string, perPage int, cond github.Conditional) (core.Page[core.Workflow], github.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("ListWorkflows", "%s cursor=%s per_page=%d%s", r, cursor, perPage, condMark(cond)); err != nil {
		return core.Page[core.Workflow]{}, github.Response{}, err
	}
	v, res := answer(core.Page[core.Workflow]{Items: slices.Clone(f.workflows)}, cond)
	return v, res, nil
}

func (f *fakeGitHub) ListJobs(_ context.Context, r core.RepoRef, runID int64, attempt int, cursor string, perPage int, cond github.Conditional) (core.Page[core.Job], github.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("ListJobs", "%s %d attempt=%d cursor=%s per_page=%d%s", r, runID, attempt, cursor, perPage, condMark(cond)); err != nil {
		return core.Page[core.Job]{}, github.Response{}, err
	}
	var items []core.Job
	for i := range f.jobs[runID] {
		if j := &f.jobs[runID][i]; attempt == 0 || j.Attempt == attempt {
			items = append(items, *j)
		}
	}
	start, _ := strconv.Atoi(cursor)
	start = min(start, len(items))
	end := min(start+perPage, len(items))
	page := core.Page[core.Job]{Items: items[start:end]}
	if end < len(items) {
		page.Next = strconv.Itoa(end)
	}
	v, res := answer(page, cond)
	return v, res, nil
}

func (f *fakeGitHub) job(jobID int64) (core.Job, bool) {
	for _, jobs := range f.jobs {
		if i := slices.IndexFunc(jobs, func(j core.Job) bool { return j.ID == jobID }); i >= 0 {
			return jobs[i], true
		}
	}
	return core.Job{}, false
}

func (f *fakeGitHub) GetJob(_ context.Context, r core.RepoRef, jobID int64, cond github.Conditional) (core.Job, github.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("GetJob", "%s %d%s", r, jobID, condMark(cond)); err != nil {
		return core.Job{}, github.Response{}, err
	}
	j, ok := f.job(jobID)
	if !ok {
		return core.Job{}, github.Response{}, errNotFound
	}
	v, res := answer(j, cond)
	return v, res, nil
}

func (f *fakeGitHub) JobLog(_ context.Context, r core.RepoRef, jobID, limit int64) (text []byte, truncated bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("JobLog", "%s %d limit=%d", r, jobID, limit); err != nil {
		return nil, false, err
	}
	if j, ok := f.job(jobID); !ok || !j.Done() {
		return nil, false, core.ErrLogPending
	}
	return []byte(f.logs[jobID]), f.truncated, nil
}

func (f *fakeGitHub) ListAnnotations(_ context.Context, r core.RepoRef, checkRunID int64, cursor string, perPage int, cond github.Conditional) (core.Page[core.Annotation], github.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("ListAnnotations", "%s %d cursor=%s per_page=%d%s", r, checkRunID, cursor, perPage, condMark(cond)); err != nil {
		return core.Page[core.Annotation]{}, github.Response{}, err
	}
	v, res := answer(core.Page[core.Annotation]{Items: slices.Clone(f.notes[checkRunID])}, cond)
	return v, res, nil
}

func (f *fakeGitHub) PullChecks(_ context.Context, r core.RepoRef, number int) (core.Checks, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("PullChecks", "%s #%d", r, number); err != nil {
		return core.Checks{}, err
	}
	return f.checks["#"+strconv.Itoa(number)], nil
}

func (f *fakeGitHub) CommitChecks(_ context.Context, r core.RepoRef, sha string) (core.Checks, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("CommitChecks", "%s @%s", r, sha); err != nil {
		return core.Checks{}, err
	}
	return f.checks["@"+sha], nil
}

// rerun starts the next attempt of run runID, whose jobs that match
// again start queued. f.mu must be held.
func (f *fakeGitHub) rerun(runID int64, again func(core.Job) bool) error {
	i := slices.IndexFunc(f.runs, func(run core.Run) bool { return run.ID == runID })
	if i < 0 {
		return errNotFound
	}
	run := &f.runs[i]
	run.Attempt++
	run.Status, run.Conclusion = core.RunQueued, core.ConclusionNone
	var next []core.Job
	for i := range f.jobs[runID] {
		j := f.jobs[runID][i]
		if j.Attempt != run.Attempt-1 {
			continue
		}
		j.Attempt = run.Attempt
		if again(j) {
			j.ID += 1000
			j.Status, j.Conclusion, j.Steps = core.RunQueued, core.ConclusionNone, nil
		}
		next = append(next, j)
	}
	f.jobs[runID] = append(f.jobs[runID], next...)
	return nil
}

func (f *fakeGitHub) RerunRun(_ context.Context, r core.RepoRef, runID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("RerunRun", "%s %d", r, runID); err != nil {
		return err
	}
	return f.rerun(runID, func(core.Job) bool { return true })
}

func (f *fakeGitHub) RerunFailedJobs(_ context.Context, r core.RepoRef, runID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("RerunFailedJobs", "%s %d", r, runID); err != nil {
		return err
	}
	return f.rerun(runID, func(j core.Job) bool { return j.Conclusion.Failed() })
}

func (f *fakeGitHub) RerunJob(_ context.Context, r core.RepoRef, jobID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("RerunJob", "%s %d", r, jobID); err != nil {
		return err
	}
	j, ok := f.job(jobID)
	if !ok {
		return errNotFound
	}
	return f.rerun(j.RunID, func(o core.Job) bool { return o.ID == jobID })
}

func (f *fakeGitHub) CancelRun(_ context.Context, r core.RepoRef, runID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("CancelRun", "%s %d", r, runID); err != nil {
		return err
	}
	// GitHub cancels a moment later.
	return nil
}

// queueJobs adds n queued jobs to run 2, which is in progress, from ID from
// on.
func (f *fakeGitHub) queueJobs(from int64, n int) {
	f.change(func(f *fakeGitHub) {
		for id := range int64(n) {
			f.jobs[2] = append(f.jobs[2], core.Job{ID: from + id, RunID: 2, Attempt: 1, Name: "shard " + strconv.FormatInt(from+id, 10), Status: core.RunQueued})
		}
	})
}
