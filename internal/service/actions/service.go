// Package actions serves GitHub Actions: the workflow runs of a
// repository, the jobs and steps of a run, the logs of jobs, and the
// checks of a pull request or commit.
//
// What changes is revalidated with ETags, which costs no rate limit while
// nothing moved: the pages of runs, a run, the jobs of a run in progress,
// the workflows and annotations. The first pages of runs and the
// workflows are kept on a shelf of the account's own for the revalidator.
// What can't change any more is kept for good: the jobs of an attempt of a
// run that completed, and the log of a job that completed, which GitHub
// publishes only then.
package actions

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// API is the part of the GitHub client that the service uses.
type API interface {
	ListRuns(ctx context.Context, repo core.RepoRef, f core.RunFilter, cursor string, perPage int, cond github.Conditional) (core.Page[core.Run], github.Response, error)
	GetRun(ctx context.Context, repo core.RepoRef, runID int64, cond github.Conditional) (core.Run, github.Response, error)
	ListWorkflows(ctx context.Context, repo core.RepoRef, cursor string, perPage int, cond github.Conditional) (core.Page[core.Workflow], github.Response, error)
	ListJobs(ctx context.Context, repo core.RepoRef, runID int64, attempt int, cursor string, perPage int, cond github.Conditional) (core.Page[core.Job], github.Response, error)
	GetJob(ctx context.Context, repo core.RepoRef, jobID int64, cond github.Conditional) (core.Job, github.Response, error)
	JobLog(ctx context.Context, repo core.RepoRef, jobID, limit int64) (text []byte, truncated bool, err error)
	ListAnnotations(ctx context.Context, repo core.RepoRef, checkRunID int64, cursor string, perPage int, cond github.Conditional) (core.Page[core.Annotation], github.Response, error)
	PullChecks(ctx context.Context, repo core.RepoRef, number int) (core.Checks, error)
	CommitChecks(ctx context.Context, repo core.RepoRef, sha string) (core.Checks, error)
	RerunRun(ctx context.Context, repo core.RepoRef, runID int64) error
	RerunFailedJobs(ctx context.Context, repo core.RepoRef, runID int64) error
	RerunJob(ctx context.Context, repo core.RepoRef, jobID int64) error
	CancelRun(ctx context.Context, repo core.RepoRef, runID int64) error
}

// forever is the TTL of what can't change.
const forever = time.Duration(math.MaxInt64)

// Service reads and changes GitHub Actions through caches. It is safe for
// concurrent use.
type Service struct {
	api API

	runs      *cache.Cache[core.Page[core.Run]]
	run       *cache.Cache[core.Run]
	workflows *cache.Cache[core.Page[core.Workflow]]
	// liveJobs holds the jobs of attempts that may still change, and
	// doneJobs those of attempts that completed.
	liveJobs    *cache.Cache[core.Page[core.Job]]
	doneJobs    *cache.Cache[core.Page[core.Job]]
	logs        *cache.Cache[core.Log]
	checks      *cache.Cache[core.Checks]
	annotations *cache.Cache[core.Page[core.Annotation]]

	keptRuns      *cache.Shelf[core.Page[core.Run]]
	keptWorkflows *cache.Shelf[core.Page[core.Workflow]]
	keptJobs      *cache.Shelf[core.Page[core.Job]]
	// store keeps the logs, which are text rather than JSON.
	store    cache.Store
	logLimit int64
}

// The kinds of entries the service keeps, and the version of their values.
// Bump schema when the core types they hold change shape.
const (
	kindRuns      = "runlist"
	kindWorkflows = "workflowlist"
	kindJobs      = "jobpage"
	kindLog       = "joblog"
	schema        = 1
)

// New returns a service that calls api.
func New(api API, opts ...Option) *Service {
	o := options{liveTTL: DefaultLiveTTL, logMemory: DefaultLogMemory, logLimit: github.DefaultLogLimit}
	for _, opt := range opts {
		opt(&o)
	}
	std := []cache.Option{cache.WithTTL(o.ttl), cache.WithCapacity(o.capacity)}
	return &Service{
		api:         api,
		runs:        cache.New[core.Page[core.Run]](std...),
		run:         cache.New[core.Run](std...),
		workflows:   cache.New[core.Page[core.Workflow]](std...),
		liveJobs:    cache.New[core.Page[core.Job]](cache.WithTTL(o.liveTTL), cache.WithCapacity(o.capacity)),
		doneJobs:    cache.New[core.Page[core.Job]](cache.WithTTL(forever), cache.WithCapacity(o.capacity)),
		logs:        cache.New[core.Log](cache.WithTTL(forever), cache.WithMaxSize(o.logMemory, logSize)),
		checks:      cache.New[core.Checks](std...),
		annotations: cache.New[core.Page[core.Annotation]](std...),

		keptRuns:      cache.NewShelf[core.Page[core.Run]](o.store, kindRuns, schema),
		keptWorkflows: cache.NewShelf[core.Page[core.Workflow]](o.store, kindWorkflows, schema),
		keptJobs:      cache.NewShelf[core.Page[core.Job]](o.store, kindJobs, schema),
		store:         o.store,
		logLimit:      o.logLimit,
	}
}

// Invalidate marks what the service holds of repo that may change stale,
// so the next read of each asks GitHub, with a conditional request. The
// jobs of completed attempts and the logs of completed jobs are kept.
func (s *Service) Invalidate(repo core.RepoRef) {
	tag := repoTag(repo)
	s.runs.InvalidateTag(tag)
	s.run.InvalidateTag(tag)
	s.workflows.InvalidateTag(tag)
	s.liveJobs.InvalidateTag(tag)
	s.checks.InvalidateTag(tag)
	s.annotations.InvalidateTag(tag)
}

// SyncKey names changes to the runs of repo in sync events, such as those
// the revalidator finds.
func SyncKey(repo core.RepoRef) string {
	return "actions:" + repoID(repo)
}

// offlineAt is when an entry served offline was fetched, as far as the
// cache can tell: long ago, so it is stale at once and the next read asks
// GitHub again.
var offlineAt = time.Unix(1, 0)

// fetch reads key from c, or loads it with load when it is missing or
// stale. A stale entry's validators make the request conditional. What
// GitHub sends is kept on shelf too, if there is one, and what GitHub
// refuses is dropped from it. If GitHub can't be reached, the stale entry
// is served instead, marked by offline.
func fetch[V any](ctx context.Context, c *cache.Cache[V], shelf *cache.Shelf[V], key string, tags []string, offline func(V) V,
	load func(ctx context.Context, cond github.Conditional) (V, github.Response, error),
) (cache.Entry[V], error) {
	return c.Fetch(ctx, key, func(ctx context.Context, prev cache.Entry[V], ok bool) (cache.Entry[V], error) {
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		v, res, err := load(ctx, cond)
		switch {
		case ok && github.Unreachable(ctx, err):
			prev.Value, prev.FetchedAt = offline(prev.Value), offlineAt
			return prev, nil
		case err != nil:
			if github.Refused(err) {
				shelf.Delete(key)
			}
			return cache.Entry[V]{}, err
		case res.NotModified:
			return cache.Entry[V]{}, cache.ErrNotModified
		}
		e := cache.Entry[V]{Value: v, ETag: res.ETag, LastModified: res.LastModified, Source: res.URL, Tags: tags}
		// The shelf is only a shortcut, so a failure is ignored.
		_ = shelf.Save(key, e)
		return e, nil
	})
}

// offlinePage marks a page served offline.
func offlinePage[T any](p core.Page[T]) core.Page[T] {
	p.Offline = true
	return p
}

// asIs serves a value offline unmarked.
func asIs[V any](v V) V { return v }

// pageSize returns n, or def for 0, at most the 100 that GitHub lists.
func pageSize(n, def int) int {
	if n <= 0 {
		return def
	}
	return min(n, 100)
}

// repoID names a repository in keys and tags. GitHub ignores case in owner
// and repository names, so keys do too.
func repoID(repo core.RepoRef) string {
	return strings.ToLower(repo.String())
}

// repoTag tags every entry of a repository.
func repoTag(repo core.RepoRef) string {
	return "repo:" + repoID(repo)
}

// runTag tags the entries of a run: the run and the pages of its jobs.
func runTag(repo core.RepoRef, runID int64) string {
	return "run:" + repoID(repo) + "/" + strconv.FormatInt(runID, 10)
}

// logLineSize is what a parsed line takes besides its text: the line, and
// the time in the text of the log that its Text is cut from.
const logLineSize = int64(unsafe.Sizeof(core.LogLine{})) + int64(len("2006-01-02T15:04:05.0000000Z "))

// logSize measures a parsed log for the memory bound: its lines, and the
// text they are cut from.
func logSize(l core.Log) int64 {
	n := int64(cap(l.Lines)) * logLineSize
	for _, line := range l.Lines {
		n += int64(len(line.Text))
	}
	return n
}
