package actions

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// A re-run or a cancel shows at once in every cached copy of the run, and
// of the jobs it affects: a re-run as queued, a cancel as cancelling.
// GitHub answers them with no more than a status and acts a moment later,
// so once it accepts one, what is cached of the run is marked stale, and
// the next read brings the run as GitHub has it: the next attempt of a
// re-run, whose jobs are read anew. The jobs kept for good of the attempt
// before go back to how it ended.

// RerunRun starts the next attempt of run runID of repo, with all its
// jobs.
func (s *Service) RerunRun(repo core.RepoRef, runID int64) *optimistic.Op {
	return s.change("re-run", repo, runID, core.RunQueued, func(core.Job) bool { return true },
		func(c core.CommitCheck) bool { return c.RunID == runID },
		func(ctx context.Context) error { return s.api.RerunRun(ctx, repo, runID) })
}

// RerunFailedJobs starts the next attempt of run runID of repo with the
// jobs that failed or were cancelled, and GitHub adds those that depend on
// them.
func (s *Service) RerunFailedJobs(repo core.RepoRef, runID int64) *optimistic.Op {
	return s.change("re-run failed jobs of", repo, runID, core.RunQueued,
		func(j core.Job) bool { return j.Conclusion.Failed() || j.Conclusion == core.ConclusionCancelled },
		func(c core.CommitCheck) bool {
			return c.RunID == runID && (c.Conclusion.Failed() || c.Conclusion == core.ConclusionCancelled)
		},
		func(ctx context.Context) error { return s.api.RerunFailedJobs(ctx, repo, runID) })
}

// RerunJob starts the next attempt of run runID of repo with job jobID,
// and GitHub adds the jobs that depend on it.
func (s *Service) RerunJob(repo core.RepoRef, runID, jobID int64) *optimistic.Op {
	return s.change("re-run job of", repo, runID, core.RunQueued, func(j core.Job) bool { return j.ID == jobID },
		func(c core.CommitCheck) bool { return c.RunID == runID && c.JobID == jobID },
		func(ctx context.Context) error { return s.api.RerunJob(ctx, repo, jobID) })
}

// CancelRun cancels run runID of repo, and the jobs of it that haven't
// completed.
func (s *Service) CancelRun(repo core.RepoRef, runID int64) *optimistic.Op {
	return s.change("cancel", repo, runID, core.RunCancelling, func(j core.Job) bool { return !j.Done() }, nil,
		func(ctx context.Context) error { return s.api.CancelRun(ctx, repo, runID) })
}

// RerequestSuite asks the App of check suite suiteID of repo to run its
// checks again on the same commit. It changes nothing at once, as what the
// App does is its own to say; once GitHub accepts it, the checks cached of
// the repository are marked stale, so the next read brings the new run. They
// are matched by the repository, since the checks of a pull request are not
// keyed by commit.
func (s *Service) RerequestSuite(repo core.RepoRef, suiteID int64) *optimistic.Op {
	if err := s.refused(); err != nil {
		return optimistic.Refused(fmt.Errorf("re-request check suite %d of %s: %w", suiteID, repo, err))
	}
	return optimistic.New(func(ctx context.Context) error {
		if err := s.api.RerequestCheckSuite(ctx, repo, suiteID); err != nil {
			return fmt.Errorf("re-request check suite %d of %s: %w", suiteID, repo, err)
		}
		s.commit.invalidateTag(repoTag(repo))
		s.checks.InvalidateTag(repoTag(repo))
		return nil
	})
}

// change shows run runID of repo, and its jobs that match, with status at
// once, and returns the Op that sends the change with send. What names
// the change in errors. A change the token may not make changes nothing,
// and the Op returns why. A re-run (matchCheck is not nil) shows too in the
// checks of the commits cached of repo: the check runs that matchCheck
// picks as queued, and once GitHub accepts it, those checks and the checks
// of the pull requests are marked stale, as GitHub starts the attempt a
// moment later.
func (s *Service) change(what string, repo core.RepoRef, runID int64, status core.RunStatus,
	match func(core.Job) bool, matchCheck func(core.CommitCheck) bool, send func(ctx context.Context) error,
) *optimistic.Op {
	if err := s.refused(); err != nil {
		return optimistic.Refused(fmt.Errorf("%s run %d of %s: %w", what, runID, repo, err))
	}
	editRun := func(r core.Run) core.Run {
		r.Status = status
		if status == core.RunQueued {
			r.Conclusion = core.ConclusionNone
		}
		return r
	}
	editJob := func(j core.Job) core.Job {
		j.Status = status
		if status == core.RunQueued {
			j.Conclusion = core.ConclusionNone
		}
		return j
	}
	pages := func(p core.Page[core.Job]) (core.Page[core.Job], bool) { return replaceJobs(p, match, editJob) }

	tag := runTag(repo, runID)
	undoRuns := s.runs.MutateTag(repoTag(repo), func(p core.Page[core.Run]) (core.Page[core.Run], bool) {
		return replaceRun(p, runID, editRun)
	})
	undoRun, _ := s.run.Mutate(runKey(repo, runID), editRun)
	undoLive := s.liveJobs.MutateTag(tag, pages)
	undoDone := s.doneJobs.MutateTag(tag, pages)

	undoChecks := func() {}
	if matchCheck != nil {
		undoChecks = s.commit.mutateTag(repoTag(repo), func(c core.CommitChecks) (core.CommitChecks, bool) {
			return editChecks(c, func(r core.CommitCheck) (core.CommitCheck, bool) {
				if !matchCheck(r) {
					return r, false
				}
				r.Status, r.Conclusion, r.CompletedAt = status, core.ConclusionNone, time.Time{}
				return r, true
			})
		})
	}

	return optimistic.New(func(ctx context.Context) error {
		if err := send(ctx); err != nil {
			return fmt.Errorf("%s run %d of %s: %w", what, runID, repo, err)
		}
		undoDone()
		s.runs.InvalidateTag(repoTag(repo))
		s.run.InvalidateTag(tag)
		s.liveJobs.InvalidateTag(tag)
		if matchCheck != nil {
			s.commit.invalidateTag(repoTag(repo))
			s.checks.InvalidateTag(repoTag(repo))
		}
		return nil
	}, undoRuns, undoRun, undoLive, undoDone, undoChecks)
}

// refused returns why the token may not re-run or cancel a run, or nil
// when it may, or when that isn't known. GitHub asks for repo even in a
// public repository.
func (s *Service) refused() error {
	if s.access == nil {
		return nil
	}
	return s.access.Check(core.NeedRuns)
}

// replaceRun returns p with run id replaced by f of it, and whether p
// holds it. It copies the items rather than change the cached ones.
func replaceRun(p core.Page[core.Run], id int64, f func(core.Run) core.Run) (core.Page[core.Run], bool) {
	i := slices.IndexFunc(p.Items, func(r core.Run) bool { return r.ID == id })
	if i < 0 {
		return p, false
	}
	p.Items = slices.Clone(p.Items)
	p.Items[i] = f(p.Items[i])
	return p, true
}

// replaceJobs returns p with the jobs that match replaced by f of them,
// and whether any did. It copies the items rather than change the cached
// ones.
func replaceJobs(p core.Page[core.Job], match func(core.Job) bool, f func(core.Job) core.Job) (core.Page[core.Job], bool) {
	if !slices.ContainsFunc(p.Items, match) {
		return p, false
	}
	p.Items = slices.Clone(p.Items)
	for i := range p.Items {
		if match(p.Items[i]) {
			p.Items[i] = f(p.Items[i])
		}
	}
	return p, true
}
