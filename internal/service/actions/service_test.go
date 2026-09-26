package actions

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

func openStore(t *testing.T) cache.Store {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func checkCalls(t *testing.T, f *fakeGitHub, want ...string) {
	t.Helper()
	if got := f.take(); !slices.Equal(got, want) {
		t.Errorf("calls = %q\nwant    %q", got, want)
	}
}

func runIDs(p core.Page[core.Run]) []int64 {
	ids := make([]int64, len(p.Items))
	for i := range p.Items {
		ids[i] = p.Items[i].ID
	}
	return ids
}

func TestRuns(t *testing.T) {
	f := newFake()
	s := New(f)
	q := RunsQuery{Repo: repo}

	if _, ok := s.CachedRuns(q); ok {
		t.Fatal("CachedRuns before a read reports a page")
	}
	p, err := s.Runs(t.Context(), q)
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if !slices.Equal(runIDs(p), []int64{2, 1}) || p.Stale || p.Offline {
		t.Errorf("page = %+v, want runs 2 and 1", p)
	}
	checkCalls(t, f, "ListRuns octo-org/hello status= cursor= per_page=30")

	if _, err := s.Runs(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f)
	if c, ok := s.CachedRuns(q); !ok || !slices.Equal(runIDs(c), []int64{2, 1}) {
		t.Errorf("CachedRuns = %+v, %v", c, ok)
	}

	// Stale, it is revalidated: first for free, then with the change.
	s.Invalidate(repo)
	if _, err := s.Runs(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, "ListRuns octo-org/hello status= cursor= per_page=30 if-none-match")
	f.change(func(f *fakeGitHub) { f.runs[0].Status = core.RunCompleted })
	s.Invalidate(repo)
	p, _ = s.Runs(t.Context(), q)
	if !p.Items[0].Done() {
		t.Errorf("run 2 = %+v after it completed", p.Items[0])
	}
}

func TestRunsFilterAndPages(t *testing.T) {
	f := newFake()
	s := New(f)
	q := RunsQuery{Repo: repo, Filter: core.RunFilter{Status: "failure"}, PageSize: 1}
	p, err := s.Runs(t.Context(), q)
	if err != nil || !slices.Equal(runIDs(p), []int64{1}) || p.Next != "" {
		t.Errorf("failing runs = %+v, %v; want run 1 alone", p, err)
	}
	p, _ = s.Runs(t.Context(), RunsQuery{Repo: repo, PageSize: 1})
	next, _ := s.Runs(t.Context(), RunsQuery{Repo: repo, PageSize: 1, Cursor: p.Next})
	if !slices.Equal(runIDs(p), []int64{2}) || !slices.Equal(runIDs(next), []int64{1}) {
		t.Errorf("pages = %v, %v; want run 2, then run 1", runIDs(p), runIDs(next))
	}
	checkCalls(t, f,
		"ListRuns octo-org/hello status=failure cursor= per_page=1",
		"ListRuns octo-org/hello status= cursor= per_page=1",
		"ListRuns octo-org/hello status= cursor=1 per_page=1",
	)
}

func TestRunsOfflineAndRefused(t *testing.T) {
	store := openStore(t)
	f := newFake()
	s := New(f, WithStore(store))
	q := RunsQuery{Repo: repo}
	if _, err := s.Runs(t.Context(), q); err != nil {
		t.Fatal(err)
	}

	f.fail(errDial)
	s.Invalidate(repo)
	p, err := s.Runs(t.Context(), q)
	if err != nil || !p.Offline || len(p.Items) != 2 {
		t.Errorf("Runs offline = %+v, %v; want the page read last, offline", p, err)
	}

	f.fail(errNotFound)
	if _, err := s.Runs(t.Context(), q); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Runs refused = %v, want ErrNotFound", err)
	}
	if _, err := New(f, WithStore(store)).Runs(t.Context(), q); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("a new session read %v, want the kept page dropped after the refusal", err)
	}
}

func TestRunsKept(t *testing.T) {
	store := openStore(t)
	f := newFake()
	q := RunsQuery{Repo: repo}
	if _, err := New(f, WithStore(store)).Runs(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	if _, err := New(f, WithStore(store)).Workflows(t.Context(), WorkflowsQuery{Repo: repo}); err != nil {
		t.Fatal(err)
	}
	f.take()

	// A later session shows the kept page at once, and asks GitHub when
	// read again.
	s := New(f, WithStore(cachetest.Aged(store, time.Hour)))
	p, err := s.Runs(t.Context(), q)
	if err != nil || !p.Stale || len(p.Items) != 2 {
		t.Errorf("Runs = %+v, %v; want the kept page, stale", p, err)
	}
	checkCalls(t, f)
	if _, err := s.Runs(t.Context(), q.again()); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, "ListRuns octo-org/hello status= cursor= per_page=30 if-none-match")

	// Only the store itself can list what it keeps.
	s = New(f, WithStore(store))
	entries := s.Kept()
	if len(entries) != 2 {
		t.Fatalf("Kept = %d entries, want the runs page and the workflows", len(entries))
	}
	f.change(func(f *fakeGitHub) { f.runs[0].Status = core.RunCompleted })
	for _, e := range entries {
		res := e.Check(t.Context())
		switch {
		case strings.HasPrefix(e.ID, kindRuns):
			if res.Status != revalidate.Changed || res.Sync != SyncKey(repo) {
				t.Errorf("check of %s = %+v, want a change of %s", e.ID, res, SyncKey(repo))
			}
		case res.Status != revalidate.NotModified:
			t.Errorf("check of %s = %+v, want it unchanged", e.ID, res)
		}
	}
	f.take()
	if p, err := New(f, WithStore(store)).Runs(t.Context(), q); err != nil || p.Stale || !p.Items[0].Done() {
		t.Errorf("kept page = %+v, %v; want the change the check found, fresh", p, err)
	}
	checkCalls(t, f)
}

func TestParseRunsKey(t *testing.T) {
	q := RunsQuery{Repo: repo, PageSize: 50, Cursor: "https://api.github.com/x?page=2", Filter: core.RunFilter{
		Branch: "feat/a b", Event: "push", Status: "failure", Actor: "octocat", HeadSHA: "abc", WorkflowID: 7,
	}}
	got, ok := parseRunsKey(runsKey(q))
	if !ok || got != q {
		t.Errorf("parseRunsKey = %+v, %v; want %+v", got, ok, q)
	}
	for _, key := range []string{"runs:bad", "runs:o/r?per_page=x", "jobs:o/r?per_page=1", "runs:o/r?per_page=1&workflow=x"} {
		if _, ok := parseRunsKey(key); ok {
			t.Errorf("parseRunsKey(%q) reports a query", key)
		}
	}
}

func TestRun(t *testing.T) {
	f := newFake()
	s := New(f)
	run, err := s.Run(t.Context(), repo, 1)
	if err != nil || run.ID != 1 || !run.Done() {
		t.Fatalf("Run = %+v, %v", run, err)
	}
	if c, ok := s.CachedRun(repo, 1); !ok || c.ID != 1 {
		t.Errorf("CachedRun = %+v, %v", c, ok)
	}
	if _, err := s.Run(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, "GetRun octo-org/hello 1")

	// A page of runs has the run too.
	s = New(f)
	if _, err := s.Runs(t.Context(), RunsQuery{Repo: repo}); err != nil {
		t.Fatal(err)
	}
	if c, ok := s.CachedRun(repo, 2); !ok || c.ID != 2 {
		t.Errorf("CachedRun from a page = %+v, %v", c, ok)
	}
	if _, ok := s.CachedRun(repo, 3); ok {
		t.Error("CachedRun reports a run nothing read")
	}
}

func TestWorkflows(t *testing.T) {
	f := newFake()
	s := New(f)
	p, err := s.Workflows(t.Context(), WorkflowsQuery{Repo: repo})
	if err != nil || len(p.Items) != 1 || p.Items[0].ID != 7 {
		t.Fatalf("Workflows = %+v, %v", p, err)
	}
	if c, ok := s.CachedWorkflows(repo); !ok || len(c.Items) != 1 {
		t.Errorf("CachedWorkflows = %+v, %v", c, ok)
	}
	checkCalls(t, f, "ListWorkflows octo-org/hello cursor= per_page=100")
}

func TestJobsOfCompletedAttemptKeptForGood(t *testing.T) {
	store := openStore(t)
	f := newFake()
	s := New(f, WithStore(store))
	if _, err := s.Run(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	q := JobsQuery{Repo: repo, RunID: 1, Attempt: 1}
	p, err := s.Jobs(t.Context(), q)
	if err != nil || len(p.Items) != 2 || len(p.Items[1].Steps) != 2 {
		t.Fatalf("Jobs = %+v, %v", p, err)
	}
	checkCalls(t, f, "GetRun octo-org/hello 1", "ListJobs octo-org/hello 1 attempt=1 cursor= per_page=100")

	// Neither staleness nor a new session asks GitHub again.
	s.Invalidate(repo)
	if _, err := s.Jobs(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	s2 := New(f, WithStore(store))
	p, err = s2.Jobs(t.Context(), q)
	if err != nil || len(p.Items) != 2 {
		t.Fatalf("Jobs of a new session = %+v, %v", p, err)
	}
	if c, ok := s2.CachedJobs(q); !ok || len(c.Items) != 2 {
		t.Errorf("CachedJobs = %+v, %v", c, ok)
	}
	checkCalls(t, f)
}

func TestJobsInProgressRevalidated(t *testing.T) {
	store := openStore(t)
	f := newFake()
	s := New(f, WithStore(store))
	if _, err := s.Run(t.Context(), repo, 2); err != nil {
		t.Fatal(err)
	}
	f.take()
	for _, q := range []JobsQuery{{Repo: repo, RunID: 2, Attempt: 1}, {Repo: repo, RunID: 1}} {
		if _, err := s.Jobs(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		s.Invalidate(repo)
		if _, err := s.Jobs(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	checkCalls(t, f,
		"ListJobs octo-org/hello 2 attempt=1 cursor= per_page=100",
		"ListJobs octo-org/hello 2 attempt=1 cursor= per_page=100 if-none-match",
		// The latest attempt changes with a re-run, even of a run that completed.
		"ListJobs octo-org/hello 1 attempt=0 cursor= per_page=100",
		"ListJobs octo-org/hello 1 attempt=0 cursor= per_page=100 if-none-match",
	)
	if _, ok := New(f, WithStore(store)).CachedJobs(JobsQuery{Repo: repo, RunID: 2, Attempt: 1}); ok {
		t.Error("a new session has the jobs of a run in progress")
	}

	// Once the run completed, its jobs are kept for good.
	f.change(func(f *fakeGitHub) {
		f.runs[0].Status, f.runs[0].Conclusion = core.RunCompleted, core.ConclusionSuccess
		f.jobs[2][1] = doneJob(22, 2, core.ConclusionSuccess)
	})
	s.Invalidate(repo)
	if _, err := s.Run(t.Context(), repo, 2); err != nil {
		t.Fatal(err)
	}
	q := JobsQuery{Repo: repo, RunID: 2, Attempt: 1}
	if p, err := s.Jobs(t.Context(), q); err != nil || !allDone(p.Items) {
		t.Fatalf("Jobs = %+v, %v; want them completed", p, err)
	}
	f.take()
	if _, err := New(f, WithStore(store)).Jobs(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f)
}

func TestLog(t *testing.T) {
	store := openStore(t)
	f := newFake()
	s := New(f, WithStore(store), WithLogLimit(1<<20))
	if _, err := s.Jobs(t.Context(), JobsQuery{Repo: repo, RunID: 1, Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	f.take()

	l, err := s.Log(t.Context(), repo, 12)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	checkLog(t, l)
	// The job was cached, so only the log is read.
	checkCalls(t, f, "JobLog octo-org/hello 12 limit=1048576")
	if c, ok := s.CachedLog(repo, 12); !ok || len(c.Lines) != 3 {
		t.Errorf("CachedLog = %+v, %v", c, ok)
	}
	if _, err := s.Log(t.Context(), repo, 12); err != nil {
		t.Fatal(err)
	}

	// A new session reads it from the store, with the steps of its lines.
	l, err = New(f, WithStore(store)).Log(t.Context(), repo, 12)
	if err != nil {
		t.Fatalf("Log of a new session: %v", err)
	}
	checkLog(t, l)
	checkCalls(t, f)
}

func checkLog(t *testing.T, l core.Log) {
	t.Helper()
	want := []core.LogLine{
		{Text: "setting up", Step: 1},
		{Text: "Run go test", Kind: core.LogGroup, Step: 2},
		{Text: "FAIL", Kind: core.LogError, Step: 2},
	}
	if len(l.Lines) != len(want) || l.Truncated {
		t.Fatalf("log = %+v, want %d lines", l, len(want))
	}
	for i, line := range l.Lines {
		if line.Text != want[i].Text || line.Kind != want[i].Kind || line.Step != want[i].Step || line.Time.IsZero() {
			t.Errorf("line %d = %+v, want %+v", i, line, want[i])
		}
	}
}

func TestLogOfJobInProgress(t *testing.T) {
	f := newFake()
	s := New(f)
	if _, err := s.Log(t.Context(), repo, 22); !errors.Is(err, core.ErrLogPending) {
		t.Errorf("Log = %v, want ErrLogPending", err)
	}
	// The job is asked for, not its log, which isn't there yet.
	checkCalls(t, f, "GetJob octo-org/hello 22")
	if _, ok := s.CachedLog(repo, 22); ok {
		t.Error("CachedLog has the log of a job in progress")
	}

	// The job completes: its log is there, though the cached job is old.
	if _, err := s.Jobs(t.Context(), JobsQuery{Repo: repo, RunID: 2}); err != nil {
		t.Fatal(err)
	}
	f.change(func(f *fakeGitHub) {
		f.jobs[2][1] = doneJob(22, 2, core.ConclusionSuccess)
		f.logs[22] = "2026-09-22T10:00:00Z done\n"
		f.truncated = true
	})
	f.take()
	l, err := s.Log(t.Context(), repo, 22)
	if err != nil || len(l.Lines) != 1 || !l.Truncated {
		t.Errorf("Log = %+v, %v; want the one line, truncated", l, err)
	}
	checkCalls(t, f, "GetJob octo-org/hello 22", "JobLog octo-org/hello 22 limit=10485760")
}

func TestLogFails(t *testing.T) {
	f := newFake()
	s := New(f)
	f.failCall("JobLog", core.ErrLogExpired)
	if _, err := s.Log(t.Context(), repo, 11); !errors.Is(err, core.ErrLogExpired) {
		t.Errorf("Log = %v, want ErrLogExpired", err)
	}
	f.failCall("JobLog", nil)
	if _, err := s.Log(t.Context(), repo, 11); err != nil {
		t.Errorf("Log after the failure = %v, want it read, as errors aren't cached", err)
	}
	if _, err := s.Log(t.Context(), repo, 99); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Log of a missing job = %v, want ErrNotFound", err)
	}
}

func TestLogMemoryBound(t *testing.T) {
	f := newFake()
	f.change(func(f *fakeGitHub) {
		f.logs[11] = strings.Repeat("2026-09-22T10:00:00Z a long line of output\n", 1000)
	})
	s := New(f, WithLogMemory(64<<10))
	for _, id := range []int64{11, 12, 21} {
		if _, err := s.Log(t.Context(), repo, id); err != nil {
			t.Fatal(err)
		}
	}
	if s.logs.Size() > 64<<10 {
		t.Errorf("logs take %d bytes, want at most %d", s.logs.Size(), 64<<10)
	}
	if _, ok := s.CachedLog(repo, 11); ok {
		t.Error("the large log is still cached, want it evicted by the later ones")
	}
}

func TestChecks(t *testing.T) {
	f := newFake()
	f.change(func(f *fakeGitHub) {
		f.checks["#5"] = core.Checks{SHA: "abc", State: core.ChecksFailure, Runs: []core.Check{{ID: 12, JobID: 12, RunID: 1, Name: "test"}}}
		f.checks["@abc"] = core.Checks{SHA: "abc", State: core.ChecksFailure}
	})
	s := New(f)
	pr := ChecksQuery{Repo: repo, Number: 5}
	c, err := s.Checks(t.Context(), pr)
	if err != nil || c.State != core.ChecksFailure || len(c.Runs) != 1 {
		t.Fatalf("Checks = %+v, %v", c, err)
	}
	if _, err := s.Checks(t.Context(), pr); err != nil {
		t.Fatal(err)
	}
	commit := ChecksQuery{Repo: repo, SHA: "ABC"}
	if c, err := s.Checks(t.Context(), commit); err != nil || c.SHA != "abc" {
		t.Errorf("Checks of the commit = %+v, %v", c, err)
	}
	checkCalls(t, f, "PullChecks octo-org/hello #5", "CommitChecks octo-org/hello @abc")
	if c, ok := s.CachedChecks(ChecksQuery{Repo: repo, SHA: "abc"}); !ok || c.SHA != "abc" {
		t.Errorf("CachedChecks = %+v, %v", c, ok)
	}
	if _, err := s.Checks(t.Context(), ChecksQuery{Repo: repo}); !errors.Is(err, errNoCommit) {
		t.Errorf("Checks of nothing = %v", err)
	}

	s.Invalidate(repo)
	if _, err := s.Checks(t.Context(), pr); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, "PullChecks octo-org/hello #5")
}

func TestAnnotations(t *testing.T) {
	f := newFake()
	f.change(func(f *fakeGitHub) {
		f.notes[12] = []core.Annotation{{Path: "a.go", StartLine: 3, EndLine: 3, Level: core.AnnotationFailure, Message: "unused"}}
	})
	s := New(f)
	q := AnnotationsQuery{Repo: repo, CheckRunID: 12}
	p, err := s.Annotations(t.Context(), q)
	if err != nil || len(p.Items) != 1 || p.Items[0].Message != "unused" {
		t.Fatalf("Annotations = %+v, %v", p, err)
	}
	if c, ok := s.CachedAnnotations(q); !ok || len(c.Items) != 1 {
		t.Errorf("CachedAnnotations = %+v, %v", c, ok)
	}
	s.Invalidate(repo)
	if _, err := s.Annotations(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f, "ListAnnotations octo-org/hello 12 cursor= per_page=50", "ListAnnotations octo-org/hello 12 cursor= per_page=50 if-none-match")
}
