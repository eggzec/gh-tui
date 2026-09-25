package actions

import (
	"errors"
	"fmt"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// primed returns a service that has read the runs of the fake, run 1 and
// run 2 on their own, and the jobs of their first attempts.
func primed(t *testing.T, f *fakeGitHub) *Service {
	t.Helper()
	s := New(f)
	if _, err := s.Runs(t.Context(), RunsQuery{Repo: repo}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2} {
		if _, err := s.Run(t.Context(), repo, id); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Jobs(t.Context(), JobsQuery{Repo: repo, RunID: id, Attempt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	f.take()
	return s
}

// shown returns what the caches show of run id: its status in the page of
// runs and on its own, and the statuses of the jobs of its first attempt.
func shown(t *testing.T, s *Service, id int64) (page, run core.RunStatus, jobs []core.RunStatus) {
	t.Helper()
	p, _ := s.CachedRuns(RunsQuery{Repo: repo})
	if r, ok := replaceRun(p, id, func(r core.Run) core.Run { page = r.Status; return r }); !ok {
		t.Fatalf("page %+v has no run %d", r, id)
	}
	r, _ := s.CachedRun(repo, id)
	js, _ := s.CachedJobs(JobsQuery{Repo: repo, RunID: id, Attempt: 1})
	for i := range js.Items {
		jobs = append(jobs, js.Items[i].Status)
	}
	return page, r.Status, jobs
}

func checkShown(t *testing.T, s *Service, id int64, wantRun core.RunStatus, wantJobs ...core.RunStatus) {
	t.Helper()
	page, run, jobs := shown(t, s, id)
	if page != wantRun || run != wantRun || fmt.Sprint(jobs) != fmt.Sprint(wantJobs) {
		t.Errorf("run %d shows %s in the page, %s alone, jobs %v; want %s, jobs %v", id, page, run, jobs, wantRun, wantJobs)
	}
}

const (
	queued     = core.RunQueued
	completed  = core.RunCompleted
	cancelling = core.RunCancelling
	inProgress = core.RunInProgress
)

func TestRerunOps(t *testing.T) {
	tests := []struct {
		name     string
		op       func(s *Service) *optimistic.Op
		call     string
		wantJobs []core.RunStatus
	}{
		{"run", func(s *Service) *optimistic.Op { return s.RerunRun(repo, 1) }, "RerunRun octo-org/hello 1", []core.RunStatus{queued, queued}},
		{"failed jobs", func(s *Service) *optimistic.Op { return s.RerunFailedJobs(repo, 1) }, "RerunFailedJobs octo-org/hello 1", []core.RunStatus{completed, queued}},
		{"job", func(s *Service) *optimistic.Op { return s.RerunJob(repo, 1, 11) }, "RerunJob octo-org/hello 11", []core.RunStatus{queued, completed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake()
			s := primed(t, f)
			op := tt.op(s)
			checkShown(t, s, 1, queued, tt.wantJobs...)
			if r, _ := s.CachedRun(repo, 1); r.Conclusion != core.ConclusionNone {
				t.Errorf("run shows conclusion %q while queued", r.Conclusion)
			}
			checkCalls(t, f)

			if err := op.Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			checkCalls(t, f, tt.call)
			// The first attempt ended as it did; the run moved on to the next.
			_, _, jobs := shown(t, s, 1)
			if fmt.Sprint(jobs) != fmt.Sprint([]core.RunStatus{completed, completed}) {
				t.Errorf("jobs of the first attempt show %v, want them as they ended", jobs)
			}
			run, err := s.Run(t.Context(), repo, 1)
			if err != nil || run.Attempt != 2 || run.Status != queued {
				t.Fatalf("Run = %+v, %v; want the second attempt, queued", run, err)
			}
			p, err := s.Runs(t.Context(), RunsQuery{Repo: repo})
			if err != nil || p.Items[1].Attempt != 2 {
				t.Errorf("Runs = %+v, %v; want run 1 read again", p, err)
			}
			jobsOf2, err := s.Jobs(t.Context(), JobsQuery{Repo: repo, RunID: 1, Attempt: 2})
			if err != nil || len(jobsOf2.Items) != 2 {
				t.Errorf("jobs of the second attempt = %+v, %v", jobsOf2, err)
			}
			checkCalls(t, f,
				"GetRun octo-org/hello 1 if-none-match",
				"ListRuns octo-org/hello status= cursor= per_page=30 if-none-match",
				"ListJobs octo-org/hello 1 attempt=2 cursor= per_page=100",
			)
		})
	}
}

func TestRerunRolledBack(t *testing.T) {
	f := newFake()
	s := primed(t, f)
	refused := &core.RefusedError{Action: "run can't be re-run", Reason: "it was created over a month ago", Err: core.ErrConflict}
	f.failCall("RerunRun", refused)

	op := s.RerunRun(repo, 1)
	checkShown(t, s, 1, queued, queued, queued)
	err := op.Do(t.Context())
	if e, ok := errors.AsType[*core.RefusedError](err); !ok || e.Reason != refused.Reason {
		t.Fatalf("Do = %v, want the refusal", err)
	}
	checkShown(t, s, 1, completed, completed, completed)
	if r, _ := s.CachedRun(repo, 1); r.Conclusion != core.ConclusionFailure {
		t.Errorf("run shows conclusion %q after the rollback, want failure", r.Conclusion)
	}
	if err := op.Do(t.Context()); !errors.Is(err, optimistic.ErrDone) {
		t.Errorf("Do again = %v, want ErrDone", err)
	}
}

func TestCancelRun(t *testing.T) {
	f := newFake()
	s := primed(t, f)
	op := s.CancelRun(repo, 2)
	checkShown(t, s, 2, cancelling, completed, cancelling)
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	checkCalls(t, f, "CancelRun octo-org/hello 2")
	// GitHub hasn't cancelled it yet, so the run keeps showing the cancel.
	if run, err := s.Run(t.Context(), repo, 2); err != nil || run.Status != cancelling {
		t.Errorf("Run = %+v, %v; want it cancelling", run, err)
	}

	f.change(func(f *fakeGitHub) {
		f.runs[0].Status, f.runs[0].Conclusion = completed, core.ConclusionCancelled
	})
	s.Invalidate(repo)
	if run, err := s.Run(t.Context(), repo, 2); err != nil || run.Conclusion != core.ConclusionCancelled {
		t.Errorf("Run = %+v, %v; want it cancelled", run, err)
	}
}

func TestCancelRolledBack(t *testing.T) {
	f := newFake()
	s := primed(t, f)
	op := s.CancelRun(repo, 2)
	op.Rollback()
	checkShown(t, s, 2, inProgress, completed, inProgress)
	if err := op.Do(t.Context()); !errors.Is(err, optimistic.ErrDone) {
		t.Errorf("Do after Rollback = %v, want ErrDone", err)
	}
	checkCalls(t, f)
}
