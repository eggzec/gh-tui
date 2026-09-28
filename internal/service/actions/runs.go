package actions

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/fallback"
)

// RunsQuery selects a page of the workflow runs of a repository, newest
// first.
type RunsQuery struct {
	Repo   core.RepoRef
	Filter core.RunFilter
	// Cursor is the Next of the previous page, or empty for the first.
	Cursor string
	// PageSize defaults to the service's run page size and is at most 100.
	PageSize int
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

// normalize returns q with its defaults set: size is the service's run
// page size.
func (q RunsQuery) normalize(size int) RunsQuery {
	q.PageSize = pageSize(q.PageSize, size)
	return q
}

// runsKey keys a page of runs by what selects it.
func runsKey(q RunsQuery) string {
	v := url.Values{"per_page": {strconv.Itoa(q.PageSize)}}
	f := q.Filter
	for k, val := range map[string]string{
		"branch": f.Branch, "event": f.Event, "status": f.Status, "actor": f.Actor, "head_sha": f.HeadSHA, "cursor": q.Cursor,
	} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if f.WorkflowID != 0 {
		v.Set("workflow", strconv.FormatInt(f.WorkflowID, 10))
	}
	return "runs:" + repoID(q.Repo) + "?" + v.Encode()
}

// parseRunsKey returns the query that runsKey made key of.
func parseRunsKey(key string) (RunsQuery, bool) {
	rest, ok := strings.CutPrefix(key, "runs:")
	if !ok {
		return RunsQuery{}, false
	}
	name, query, _ := strings.Cut(rest, "?")
	repo, err := core.ParseRepoRef(name)
	if err != nil {
		return RunsQuery{}, false
	}
	v, err := url.ParseQuery(query)
	if err != nil {
		return RunsQuery{}, false
	}
	size, err := strconv.Atoi(v.Get("per_page"))
	if err != nil || size <= 0 {
		return RunsQuery{}, false
	}
	q := RunsQuery{Repo: repo, Cursor: v.Get("cursor"), PageSize: size, Filter: core.RunFilter{
		Branch: v.Get("branch"), Event: v.Get("event"), Status: v.Get("status"), Actor: v.Get("actor"), HeadSHA: v.Get("head_sha"),
	}}
	if w := v.Get("workflow"); w != "" {
		if q.Filter.WorkflowID, err = strconv.ParseInt(w, 10, 64); err != nil {
			return RunsQuery{}, false
		}
	}
	return q, true
}

// CachedRuns returns the cached page for q, fresh or stale, without a
// request. It reports false if the page isn't in memory.
func (s *Service) CachedRuns(q RunsQuery) (core.Page[core.Run], bool) {
	e, st := s.runs.Get(runsKey(q.normalize(s.runPageSize)))
	return e.Value, st != cache.Miss
}

// Runs returns a page of the runs of q.Repo that match q.Filter, newest
// first. A page is revalidated with its ETag. A first page kept by an
// earlier session is served at once with Stale set, until a read with
// q.Again set asks GitHub; while GitHub can't be reached, the page read
// last is served with Offline set, and while it rate limits the reads,
// with Limited set.
func (s *Service) Runs(ctx context.Context, q RunsQuery) (core.Page[core.Run], error) {
	q = q.normalize(s.runPageSize)
	key := runsKey(q)
	// Later pages shift as runs start, so only first pages are kept.
	var shelf *cache.Shelf[core.Page[core.Run]]
	if q.Cursor == "" {
		shelf = s.keptRuns
		if e, ok := shelf.Warm(s.runs, key, q.Again); ok {
			p := e.Value
			p.Stale = true
			return p, nil
		}
	}
	e, err := fetch(ctx, s.runs, shelf, key, []string{repoTag(q.Repo)}, fallback.Page[core.Run], s.loadRuns(q))
	if err != nil {
		return core.Page[core.Run]{}, fmt.Errorf("list runs of %s: %w", q.Repo, err)
	}
	return e.Value, nil
}

func (s *Service) loadRuns(q RunsQuery) func(ctx context.Context, cond github.Conditional) (core.Page[core.Run], github.Response, error) {
	return func(ctx context.Context, cond github.Conditional) (core.Page[core.Run], github.Response, error) {
		return s.api.ListRuns(ctx, q.Repo, q.Filter, q.Cursor, q.PageSize, cond)
	}
}

func runKey(repo core.RepoRef, runID int64) string {
	return "run:" + repoID(repo) + "/" + strconv.FormatInt(runID, 10)
}

// CachedRun returns run runID of repo from memory, from the run read on
// its own or from a cached page of runs, without a request. It reports
// false if neither holds it.
func (s *Service) CachedRun(repo core.RepoRef, runID int64) (core.Run, bool) {
	if e, st := s.run.Get(runKey(repo, runID)); st != cache.Miss {
		return e.Value, true
	}
	for _, p := range s.runs.Tagged(repoTag(repo)) {
		if i := slices.IndexFunc(p.Items, func(r core.Run) bool { return r.ID == runID }); i >= 0 {
			return p.Items[i], true
		}
	}
	return core.Run{}, false
}

// Run returns the latest attempt of run runID of repo, revalidated with
// its ETag, such as to open a run that a check of a pull request names.
func (s *Service) Run(ctx context.Context, repo core.RepoRef, runID int64) (core.Run, error) {
	e, err := fetch(ctx, s.run, nil, runKey(repo, runID), []string{repoTag(repo), runTag(repo, runID)}, fallback.None[core.Run],
		func(ctx context.Context, cond github.Conditional) (core.Run, github.Response, error) {
			return s.api.GetRun(ctx, repo, runID, cond)
		})
	if err != nil {
		return core.Run{}, fmt.Errorf("get run %d of %s: %w", runID, repo, err)
	}
	return e.Value, nil
}

func workflowsKey(repo core.RepoRef) string {
	return "workflows:" + repoID(repo)
}

// workflowsPageSize is how many workflows a repository lists on one page:
// all of them, for all but the largest.
const workflowsPageSize = 100

// CachedWorkflows returns the cached workflows of repo, fresh or stale,
// without a request. It reports false if they aren't in memory.
func (s *Service) CachedWorkflows(repo core.RepoRef) (core.Page[core.Workflow], bool) {
	e, st := s.workflows.Get(workflowsKey(repo))
	return e.Value, st != cache.Miss
}

// WorkflowsQuery selects the workflows of a repository.
type WorkflowsQuery struct {
	Repo core.RepoRef
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

// Workflows returns the first hundred workflows of q.Repo, to filter runs
// by, revalidated and kept like the first page of runs.
func (s *Service) Workflows(ctx context.Context, q WorkflowsQuery) (core.Page[core.Workflow], error) {
	repo := q.Repo
	key := workflowsKey(repo)
	if e, ok := s.keptWorkflows.Warm(s.workflows, key, q.Again); ok {
		p := e.Value
		p.Stale = true
		return p, nil
	}
	e, err := fetch(ctx, s.workflows, s.keptWorkflows, key, []string{repoTag(repo)}, fallback.Page[core.Workflow], s.loadWorkflows(repo))
	if err != nil {
		return core.Page[core.Workflow]{}, fmt.Errorf("list workflows of %s: %w", repo, err)
	}
	return e.Value, nil
}

func (s *Service) loadWorkflows(repo core.RepoRef) func(ctx context.Context, cond github.Conditional) (core.Page[core.Workflow], github.Response, error) {
	return func(ctx context.Context, cond github.Conditional) (core.Page[core.Workflow], github.Response, error) {
		return s.api.ListWorkflows(ctx, repo, "", workflowsPageSize, cond)
	}
}
