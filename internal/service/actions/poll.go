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
// runs. Each poll revalidates the run and the jobs of its latest attempt
// with their ETags, two requests that cost no rate limit while nothing
// moved, and stores what changed in the cache, where the next reads find
// it. It reports a change when either did, and the run in the cached
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

		q := JobsQuery{Repo: repo, RunID: runID, Attempt: run.Attempt}.normalize()
		jkey := jobsKey(q)
		jobsBefore, _ := s.liveJobs.Get(jkey)
		s.liveJobs.Invalidate(jkey)
		if _, err := s.Jobs(ctx, q); err != nil {
			return watch.Result{}, fmt.Errorf("poll run %d of %s: %w", runID, repo, err)
		}
		jobsAfter, _ := s.liveJobs.Get(jkey)
		changed = changed || jobsBefore.ETag != jobsAfter.ETag

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
