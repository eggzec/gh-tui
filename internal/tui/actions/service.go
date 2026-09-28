package actions

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// Service is what the modal needs from the actions service.
type Service interface {
	Runs(ctx context.Context, q actionssvc.RunsQuery) (core.Page[core.Run], error)
	// CachedRun returns a run from memory, without a request, as a change
	// or a poll left it.
	CachedRun(repo core.RepoRef, runID int64) (core.Run, bool)
	Run(ctx context.Context, repo core.RepoRef, runID int64) (core.Run, error)
	Workflows(ctx context.Context, q actionssvc.WorkflowsQuery) (core.Page[core.Workflow], error)
	// CachedAllJobs returns the jobs of an attempt from memory, without a
	// request.
	CachedAllJobs(q actionssvc.JobsQuery) (core.Page[core.Job], bool)
	// AllJobs returns the jobs of an attempt, every page of them up to
	// actionssvc.MaxJobPages, with Next set past that.
	AllJobs(ctx context.Context, q actionssvc.JobsQuery) (core.Page[core.Job], error)
	// CachedLog returns a job's log from memory, without a request.
	CachedLog(repo core.RepoRef, jobID int64) (core.Log, bool)
	Log(ctx context.Context, repo core.RepoRef, jobID int64) (core.Log, error)
	// CachedAnnotations returns a page of the annotations of a job from
	// memory, without a request.
	CachedAnnotations(q actionssvc.AnnotationsQuery) (core.Page[core.Annotation], bool)
	Annotations(ctx context.Context, q actionssvc.AnnotationsQuery) (core.Page[core.Annotation], error)

	RerunRun(repo core.RepoRef, runID int64) *optimistic.Op
	RerunFailedJobs(repo core.RepoRef, runID int64) *optimistic.Op
	RerunJob(repo core.RepoRef, runID, jobID int64) *optimistic.Op
	CancelRun(repo core.RepoRef, runID int64) *optimistic.Op
}
