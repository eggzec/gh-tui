package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/eggzec/gh-tui/internal/core"
)

// The mutations of runs return nothing but a status, 201 for a re-run and
// 202 for a cancel: GitHub starts the new attempt, or cancels the run, a
// moment later. A caller reads the run again to see it.

// RerunRun starts the next attempt of run runID of repo, with all its
// jobs.
func (c *Client) RerunRun(ctx context.Context, repo core.RepoRef, runID int64) error {
	return c.runAction(ctx, runPath(repo, runID)+"/rerun", "run can't be re-run")
}

// RerunFailedJobs starts the next attempt of run runID of repo with the
// jobs that failed and those that depend on them.
func (c *Client) RerunFailedJobs(ctx context.Context, repo core.RepoRef, runID int64) error {
	return c.runAction(ctx, runPath(repo, runID)+"/rerun-failed-jobs", "failed jobs can't be re-run")
}

// RerunJob starts the next attempt of the run of job jobID of repo with
// the job and those that depend on it.
func (c *Client) RerunJob(ctx context.Context, repo core.RepoRef, jobID int64) error {
	return c.runAction(ctx, jobPath(repo, jobID)+"/rerun", "job can't be re-run")
}

// CancelRun asks GitHub to cancel run runID of repo, which it does once
// the jobs running stop.
func (c *Client) CancelRun(ctx context.Context, repo core.RepoRef, runID int64) error {
	return c.runAction(ctx, runPath(repo, runID)+"/cancel", "run can't be cancelled")
}

// runAction posts to path. When GitHub refuses, as it does to re-run a
// run older than a month or to cancel one that completed, the error is a
// *core.RefusedError of refused with GitHub's reason.
func (c *Client) runAction(ctx context.Context, path, refused string) error {
	_, err := c.Do(ctx, http.MethodPost, path, nil, nil)
	if err == nil {
		return nil
	}
	if e, ok := errors.AsType[*Error](err); ok && runRefusal(e) {
		return &core.RefusedError{Action: refused, Reason: e.Message, Err: err}
	}
	return fmt.Errorf("%s: %w", refused, err)
}

// runRefusal reports whether e refuses an action on a run for a reason
// the user should see, rather than for a rate limit.
func runRefusal(e *Error) bool {
	switch e.StatusCode {
	case http.StatusForbidden:
		return !errors.Is(e, core.ErrRateLimited)
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}
