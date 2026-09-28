package actions

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/watch"
)

// RunSyncKey is the sync key under which the app subscribes Poll of run
// runID of repo.
func RunSyncKey(repo core.RepoRef, runID int64) string {
	return SyncKey(repo) + "/run/" + strconv.FormatInt(runID, 10)
}

// Poll returns a watch.PollFunc that follows run runID of repo while it
// runs. Each poll revalidates the run and the pages of jobs of its latest
// attempt with their ETags, requests that cost no rate limit while nothing
// moved, and stores what changed in the cache, where the next reads find
// it. It reports a change when any did, and the run in the cached
// pages of runs is replaced, which are marked stale besides, as the run
// may have left a filter such as the one of runs in progress. Once the run
// completed, the poll reports that change and then asks nothing more.
func (s *Service) Poll(repo core.RepoRef, runID int64) watch.PollFunc {
	var done atomic.Bool
	return func(ctx context.Context) (watch.Result, error) {
		if done.Load() {
			return watch.Result{}, nil
		}
		key := runKey(repo, runID)
		before, _ := s.run.Get(key)
		s.run.Invalidate(key)
		run, err := s.Run(ctx, repo, runID)
		if err != nil {
			return watch.Result{}, fmt.Errorf("poll run %d of %s: %w", runID, repo, err)
		}
		after, _ := s.run.Get(key)
		changed := before.ETag != after.ETag

		jobsChanged, err := s.pollJobs(ctx, JobsQuery{Repo: repo, RunID: runID, Attempt: run.Attempt})
		if err != nil {
			return watch.Result{}, fmt.Errorf("poll run %d of %s: %w", runID, repo, err)
		}
		changed = changed || jobsChanged

		if changed {
			tag := repoTag(repo)
			s.runs.MutateTag(tag, func(p core.Page[core.Run]) (core.Page[core.Run], bool) {
				return replaceRun(p, runID, func(core.Run) core.Run { return run })
			})
			s.runs.InvalidateTag(tag)
		}
		if run.Done() {
			done.Store(true)
		}
		return watch.Result{Changed: changed}, nil
	}
}

// pollJobs revalidates the pages of jobs of q that AllJobs reads, each
// with its ETag, and reports whether any changed. A page that lists more
// leads to the next, so jobs that a new page holds are read too.
func (s *Service) pollJobs(ctx context.Context, q JobsQuery) (bool, error) {
	changed := false
	_, err := allJobs(q, func(q JobsQuery) (core.Page[core.Job], error) {
		key := jobsKey(q)
		before, _ := s.liveJobs.Get(key)
		s.liveJobs.Invalidate(key)
		p, err := s.Jobs(ctx, q)
		after, _ := s.liveJobs.Get(key)
		changed = changed || before.ETag != after.ETag
		return p, err
	})
	return changed, err
}
