package actions

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// DefaultJobPageSize is the page size of a JobsQuery that sets none: all
// the jobs of most runs, matrix and all.
const DefaultJobPageSize = 100

// JobsQuery selects a page of the jobs of an attempt of a run.
type JobsQuery struct {
	Repo  core.RepoRef
	RunID int64
	// Attempt is the attempt of the run, such as the run's Attempt. The
	// jobs of an attempt that completed never change and are kept for
	// good. Zero reads the latest attempt, which a re-run replaces, so it
	// is never kept.
	Attempt int
	// Cursor is the Next of the previous page, or empty for the first.
	Cursor string
	// PageSize defaults to DefaultJobPageSize and is at most 100.
	PageSize int
}

func (q JobsQuery) normalize() JobsQuery {
	q.PageSize = pageSize(q.PageSize, DefaultJobPageSize)
	return q
}

func jobsKey(q JobsQuery) string {
	v := url.Values{"attempt": {strconv.Itoa(q.Attempt)}, "per_page": {strconv.Itoa(q.PageSize)}}
	if q.Cursor != "" {
		v.Set("cursor", q.Cursor)
	}
	return "jobs:" + repoID(q.Repo) + "/" + strconv.FormatInt(q.RunID, 10) + "?" + v.Encode()
}

// CachedJobs returns the cached page for q, fresh or stale, without a
// request. It reports false if the page isn't in memory.
func (s *Service) CachedJobs(q JobsQuery) (core.Page[core.Job], bool) {
	key := jobsKey(q.normalize())
	if e, st := s.doneJobs.Get(key); st != cache.Miss {
		return e.Value, true
	}
	e, st := s.liveJobs.Get(key)
	return e.Value, st != cache.Miss
}

// Jobs returns a page of the jobs of q.Attempt of run q.RunID, with their
// steps. The jobs of an attempt that the service knows completed, from a
// run it read, are read from memory, then from the store, and only then
// from GitHub, and kept for good. Others are revalidated with their ETag
// once they are older than the live TTL.
func (s *Service) Jobs(ctx context.Context, q JobsQuery) (core.Page[core.Job], error) {
	q = q.normalize()
	key := jobsKey(q)
	if e, st := s.doneJobs.Get(key); st != cache.Miss {
		return e.Value, nil
	}
	if q.Attempt > 0 {
		if kept, ok := s.keptJobs.Load(key); ok {
			s.doneJobs.Set(key, cache.Entry[core.Page[core.Job]]{Value: kept.Value, Tags: kept.Tags})
			return kept.Value, nil
		}
	}
	tags := []string{repoTag(q.Repo), runTag(q.Repo, q.RunID)}
	e, err := fetch(ctx, s.liveJobs, nil, key, tags, offlinePage[core.Job], func(ctx context.Context, cond github.Conditional) (core.Page[core.Job], github.Response, error) {
		return s.api.ListJobs(ctx, q.Repo, q.RunID, q.Attempt, q.Cursor, q.PageSize, cond)
	})
	if err != nil {
		return core.Page[core.Job]{}, fmt.Errorf("list jobs of run %d of %s: %w", q.RunID, q.Repo, err)
	}
	if !e.Value.Offline && s.attemptDone(q) && allDone(e.Value.Items) {
		done := cache.Entry[core.Page[core.Job]]{Value: e.Value, Tags: tags}
		s.doneJobs.Set(key, done)
		_ = s.keptJobs.Save(key, done)
	}
	return e.Value, nil
}

// attemptDone reports whether the service knows that the attempt of q
// completed, so its jobs can't change any more: a later attempt started,
// which only a completed one allows, or the run it read last completed.
func (s *Service) attemptDone(q JobsQuery) bool {
	if q.Attempt <= 0 {
		return false
	}
	run, ok := s.CachedRun(q.Repo, q.RunID)
	return ok && (run.Attempt > q.Attempt || run.Attempt == q.Attempt && run.Done())
}

func allDone(jobs []core.Job) bool {
	return !slices.ContainsFunc(jobs, func(j core.Job) bool { return !j.Done() })
}

// cachedJob returns job jobID of repo from the cached pages of jobs.
func (s *Service) cachedJob(repo core.RepoRef, jobID int64) (core.Job, bool) {
	tag := repoTag(repo)
	for _, c := range []*cache.Cache[core.Page[core.Job]]{s.doneJobs, s.liveJobs} {
		for _, p := range c.Tagged(tag) {
			if i := slices.IndexFunc(p.Items, func(j core.Job) bool { return j.ID == jobID }); i >= 0 {
				return p.Items[i], true
			}
		}
	}
	return core.Job{}, false
}
