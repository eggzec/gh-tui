package actions

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// commitChecksOf returns what the commit "abc" lists: jobs 11 and 12 of
// run 1, of which 12 failed, and job 21 of run 2, all completed.
func commitChecksOf() core.CommitChecks {
	job := func(run, id int64, c core.Conclusion) core.CommitCheck {
		return core.CommitCheck{
			RunID: run, JobID: id, Status: completed, Conclusion: c,
			CompletedAt: time.Unix(100, 0),
		}
	}
	return core.CommitChecks{
		SHA:   "abc",
		State: core.ChecksFailure,
		Runs: []core.CommitCheck{
			job(1, 11, core.ConclusionSuccess),
			job(1, 12, core.ConclusionFailure),
			job(2, 21, core.ConclusionSuccess),
		},
	}
}

var commitQuery = CommitChecksQuery{Repo: repo, SHA: "abc"}

// commitShown returns the job, status, conclusion and whether it has no end
// time, of each check the cache shows of the commit "abc".
func commitShown(t *testing.T, s *Service) []string {
	t.Helper()
	c, ok := s.CachedCommitChecks(commitQuery)
	if !ok {
		t.Fatal("the checks of the commit aren't cached")
	}
	got := make([]string, 0, len(c.Runs))
	for i := range c.Runs {
		r := &c.Runs[i]
		got = append(got, fmt.Sprintf("%d:%s/%s/%t", r.JobID, r.Status, r.Conclusion, r.CompletedAt.IsZero()))
	}
	return got
}

// primedCommit returns a service that has read the checks of the commit
// "abc", all completed, so they are kept for long.
func primedCommit(t *testing.T) (*Service, *fakeGitHub) {
	t.Helper()
	f := newFake()
	f.commits["abc"] = commitChecksOf()
	s := primed(t, f)
	if _, err := s.CommitChecks(t.Context(), commitQuery); err != nil {
		t.Fatal(err)
	}
	f.take()
	return s, f
}

const (
	job11Done  = "11:completed/success/false"
	job12Done  = "12:completed/failure/false"
	job21Done  = "21:completed/success/false"
	job11Again = "11:queued//true"
	job12Again = "12:queued//true"
)

func TestRerunShowsInCommitChecks(t *testing.T) {
	tests := []struct {
		name string
		op   func(s *Service) *optimistic.Op
		call string
		want []string
	}{
		{"run", func(s *Service) *optimistic.Op { return s.RerunRun(repo, 1) },
			"RerunRun octo-org/hello 1", []string{job11Again, job12Again, job21Done}},
		{"failed jobs", func(s *Service) *optimistic.Op { return s.RerunFailedJobs(repo, 1) },
			"RerunFailedJobs octo-org/hello 1", []string{job11Done, job12Again, job21Done}},
		{"job", func(s *Service) *optimistic.Op { return s.RerunJob(repo, 1, 11) },
			"RerunJob octo-org/hello 11", []string{job11Again, job12Done, job21Done}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, f := primedCommit(t)
			op := tt.op(s)
			if got := commitShown(t, s); fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("checks show %v, want %v", got, tt.want)
			}
			// The long-lived copy of the completed checks must not hide it.
			if c, err := s.CommitChecks(t.Context(), commitQuery); err != nil || !c.Pending() {
				t.Errorf("CommitChecks = %+v, %v; want the pending checks", c, err)
			}
			checkCalls(t, f)

			if err := op.Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if s.FreshCommitChecks(commitQuery) {
				t.Error("the checks of the commit are still fresh after the re-run")
			}
			if got := commitShown(t, s); fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("checks show %v once accepted, want them kept as pending until the next read", got)
			}
			if _, err := s.CommitChecks(t.Context(), commitQuery); err != nil {
				t.Fatal(err)
			}
			checkCalls(t, f, tt.call, "CommitChecksAll octo-org/hello @abc notes=false")
		})
	}
}

func TestRerunOfAnotherRunLeavesCommitChecks(t *testing.T) {
	s, _ := primedCommit(t)
	s.RerunJob(repo, 3, 11)
	if got := commitShown(t, s); fmt.Sprint(got) != fmt.Sprint([]string{job11Done, job12Done, job21Done}) {
		t.Errorf("checks show %v, want them as they were", got)
	}
	if !s.FreshCommitChecks(commitQuery) {
		t.Error("a re-run of another run made the checks of the commit stale")
	}
}

func TestRerunRolledBackInCommitChecks(t *testing.T) {
	s, f := primedCommit(t)
	f.failCall("RerunJob", errors.New("boom"))

	op := s.RerunJob(repo, 1, 12)
	if got := commitShown(t, s); got[1] != job12Again {
		t.Fatalf("job 12 shows %q, want it queued", got[1])
	}
	if err := op.Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the failure")
	}
	if got := commitShown(t, s); fmt.Sprint(got) != fmt.Sprint([]string{job11Done, job12Done, job21Done}) {
		t.Errorf("checks show %v after the rollback, want the old conclusions", got)
	}
	if !s.FreshCommitChecks(commitQuery) {
		t.Error("the checks aren't fresh after the rollback, want them as before")
	}
	if c, err := s.CommitChecks(t.Context(), commitQuery); err != nil || c.Pending() {
		t.Errorf("CommitChecks = %+v, %v; want the completed checks", c, err)
	}
}

func TestCancelLeavesCommitChecks(t *testing.T) {
	s, _ := primedCommit(t)
	s.CancelRun(repo, 1)
	if got := commitShown(t, s); fmt.Sprint(got) != fmt.Sprint([]string{job11Done, job12Done, job21Done}) {
		t.Errorf("checks show %v, want them as they were", got)
	}
}
