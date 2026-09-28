package github

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// GitHub Actions is read with REST: GraphQL has no workflow runs or jobs,
// and REST answers conditional requests, so polling a run in progress
// costs nothing while it doesn't move.

// actionsMaxPerPage is the most runs, jobs or workflows GitHub lists on a
// page.
const actionsMaxPerPage = 100

// restRun is the REST shape of a workflow run.
type restRun struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	DisplayTitle string    `json:"display_title"`
	RunNumber    int       `json:"run_number"`
	RunAttempt   int       `json:"run_attempt"`
	Event        string    `json:"event"`
	HeadBranch   string    `json:"head_branch"`
	HeadSHA      string    `json:"head_sha"`
	Status       string    `json:"status"`
	Conclusion   *string   `json:"conclusion"`
	WorkflowID   int64     `json:"workflow_id"`
	Actor        *user     `json:"actor"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	RunStartedAt time.Time `json:"run_started_at"`
	HTMLURL      string    `json:"html_url"`
	PullRequests []struct {
		Number int `json:"number"`
	} `json:"pull_requests"`
}

func (r restRun) core() core.Run {
	out := core.Run{
		ID:           r.ID,
		Attempt:      r.RunAttempt,
		Name:         r.Name,
		DisplayTitle: r.DisplayTitle,
		Number:       r.RunNumber,
		Event:        r.Event,
		Branch:       r.HeadBranch,
		HeadSHA:      r.HeadSHA,
		Status:       core.RunStatus(r.Status),
		Conclusion:   runConclusion(r.Conclusion),
		WorkflowID:   r.WorkflowID,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
		RunStartedAt: r.RunStartedAt,
		URL:          r.HTMLURL,
	}
	if r.Actor != nil {
		out.Actor = r.Actor.Login
	}
	for _, pr := range r.PullRequests {
		out.PullRequests = append(out.PullRequests, pr.Number)
	}
	return out
}

func runConclusion(c *string) core.Conclusion {
	if c == nil {
		return core.ConclusionNone
	}
	return core.Conclusion(*c)
}

// restJob is the REST shape of a job of a workflow run.
type restJob struct {
	ID           int64      `json:"id"`
	RunID        int64      `json:"run_id"`
	RunAttempt   int        `json:"run_attempt"`
	Name         string     `json:"name"`
	WorkflowName string     `json:"workflow_name"`
	Status       string     `json:"status"`
	Conclusion   *string    `json:"conclusion"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	Steps        []jobStep  `json:"steps"`
	HTMLURL      string     `json:"html_url"`
	RunnerName   *string    `json:"runner_name"`
	Labels       []string   `json:"labels"`
}

// jobStep is the REST shape of a step of a job.
type jobStep struct {
	Number      int        `json:"number"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  *string    `json:"conclusion"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

func (j restJob) core() core.Job {
	out := core.Job{
		ID:           j.ID,
		RunID:        j.RunID,
		Attempt:      j.RunAttempt,
		Name:         j.Name,
		WorkflowName: j.WorkflowName,
		Status:       core.RunStatus(j.Status),
		Conclusion:   runConclusion(j.Conclusion),
		StartedAt:    jobTime(j.StartedAt),
		CompletedAt:  jobTime(j.CompletedAt),
		Steps:        convert(j.Steps, jobStep.core),
		URL:          j.HTMLURL,
		Labels:       j.Labels,
	}
	if j.RunnerName != nil {
		out.RunnerName = *j.RunnerName
	}
	return out
}

func (s jobStep) core() core.Step {
	return core.Step{
		Number:      s.Number,
		Name:        s.Name,
		Status:      core.RunStatus(s.Status),
		Conclusion:  runConclusion(s.Conclusion),
		StartedAt:   jobTime(s.StartedAt),
		CompletedAt: jobTime(s.CompletedAt),
	}
}

// jobTime returns the zero time for a time GitHub left null, as it does
// for what hasn't happened yet.
func jobTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// restWorkflow is the REST shape of a workflow.
type restWorkflow struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
}

func (w restWorkflow) core() core.Workflow {
	return core.Workflow{ID: w.ID, Name: w.Name, Path: w.Path, State: w.State, URL: w.HTMLURL}
}

// ListRuns returns a page of the workflow runs of repo that match f,
// newest first. Cursor is the Next of the previous page, or empty for the
// first; perPage is at most 100, and 0 means GitHub's default of 30. If
// cond is current, the Response has NotModified set and the page is empty.
//
// A filter by workflow lists the runs of that workflow, which is another
// path. Runs are paged by number, so a run that starts between two reads
// shifts the later pages by one.
func (c *Client) ListRuns(ctx context.Context, repo core.RepoRef, f core.RunFilter, cursor string, perPage int, cond Conditional) (core.Page[core.Run], Response, error) {
	path := cursor
	if path == "" {
		path = runsPath(repo, f, perPage)
	}
	var body restRuns
	res, err := c.Get(ctx, path, cond, &body)
	switch {
	case err != nil:
		return core.Page[core.Run]{}, res, fmt.Errorf("list runs of %s: %w", repo, err)
	case res.NotModified:
		return core.Page[core.Run]{}, res, nil
	}
	return core.Page[core.Run]{Items: convert(body.WorkflowRuns, restRun.core), Next: res.Next}, res, nil
}

func runsPath(repo core.RepoRef, f core.RunFilter, perPage int) string {
	q := url.Values{}
	for k, v := range map[string]string{
		"branch": f.Branch, "event": f.Event, "status": f.Status, "actor": f.Actor, "head_sha": f.HeadSHA,
	} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if perPage > 0 {
		q.Set("per_page", strconv.Itoa(min(perPage, actionsMaxPerPage)))
	}
	path := actionsPath(repo) + "/runs"
	if f.WorkflowID != 0 {
		path = actionsPath(repo) + "/workflows/" + strconv.FormatInt(f.WorkflowID, 10) + "/runs"
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return path
}

// GetRun returns the latest attempt of run runID of repo. If cond is
// current, the Response has NotModified set and the run is empty.
func (c *Client) GetRun(ctx context.Context, repo core.RepoRef, runID int64, cond Conditional) (core.Run, Response, error) {
	var r restRun
	res, err := c.Get(ctx, runPath(repo, runID), cond, &r)
	switch {
	case err != nil:
		return core.Run{}, res, fmt.Errorf("get run %d of %s: %w", runID, repo, err)
	case res.NotModified:
		return core.Run{}, res, nil
	}
	return r.core(), res, nil
}

// ListWorkflows returns a page of the workflows of repo, to filter runs
// by. Cursor and perPage work as in ListRuns.
func (c *Client) ListWorkflows(ctx context.Context, repo core.RepoRef, cursor string, perPage int, cond Conditional) (core.Page[core.Workflow], Response, error) {
	path := cursor
	if path == "" {
		path = actionsPath(repo) + "/workflows" + perPageQuery(perPage)
	}
	var body restWorkflows
	res, err := c.Get(ctx, path, cond, &body)
	switch {
	case err != nil:
		return core.Page[core.Workflow]{}, res, fmt.Errorf("list workflows of %s: %w", repo, err)
	case res.NotModified:
		return core.Page[core.Workflow]{}, res, nil
	}
	return core.Page[core.Workflow]{Items: convert(body.Workflows, restWorkflow.core), Next: res.Next}, res, nil
}

// ListJobs returns a page of the jobs of attempt of run runID, with their
// steps. Attempt 0 means the latest attempt, whose jobs a re-run replaces.
// Cursor and perPage work as in ListRuns. If cond is current, the Response
// has NotModified set and the page is empty.
func (c *Client) ListJobs(ctx context.Context, repo core.RepoRef, runID int64, attempt int, cursor string, perPage int, cond Conditional) (core.Page[core.Job], Response, error) {
	path := cursor
	if path == "" {
		path = jobsPath(repo, runID, attempt, perPage)
	}
	var body restJobs
	res, err := c.Get(ctx, path, cond, &body)
	switch {
	case err != nil:
		return core.Page[core.Job]{}, res, fmt.Errorf("list jobs of run %d of %s: %w", runID, repo, err)
	case res.NotModified:
		return core.Page[core.Job]{}, res, nil
	}
	return core.Page[core.Job]{Items: convert(body.Jobs, restJob.core), Next: res.Next}, res, nil
}

func jobsPath(repo core.RepoRef, runID int64, attempt, perPage int) string {
	path := runPath(repo, runID)
	if attempt > 0 {
		path += "/attempts/" + strconv.Itoa(attempt) + "/jobs" + perPageQuery(perPage)
		return path
	}
	q := url.Values{"filter": {"latest"}}
	if perPage > 0 {
		q.Set("per_page", strconv.Itoa(min(perPage, actionsMaxPerPage)))
	}
	return path + "/jobs?" + q.Encode()
}

// GetJob returns job jobID of repo with its steps. If cond is current, the
// Response has NotModified set and the job is empty.
func (c *Client) GetJob(ctx context.Context, repo core.RepoRef, jobID int64, cond Conditional) (core.Job, Response, error) {
	var j restJob
	res, err := c.Get(ctx, jobPath(repo, jobID), cond, &j)
	switch {
	case err != nil:
		return core.Job{}, res, fmt.Errorf("get job %d of %s: %w", jobID, repo, err)
	case res.NotModified:
		return core.Job{}, res, nil
	}
	return j.core(), res, nil
}

func perPageQuery(perPage int) string {
	if perPage <= 0 {
		return ""
	}
	return "?per_page=" + strconv.Itoa(min(perPage, actionsMaxPerPage))
}

func actionsPath(repo core.RepoRef) string {
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) + "/actions"
}

func runPath(repo core.RepoRef, runID int64) string {
	return actionsPath(repo) + "/runs/" + strconv.FormatInt(runID, 10)
}

func jobPath(repo core.RepoRef, jobID int64) string {
	return actionsPath(repo) + "/jobs/" + strconv.FormatInt(jobID, 10)
}

// restRuns is a page of workflow runs.
type restRuns struct {
	WorkflowRuns []restRun `json:"workflow_runs"`
}

// restWorkflows is a page of workflows.
type restWorkflows struct {
	Workflows []restWorkflow `json:"workflows"`
}

// restJobs is a page of jobs.
type restJobs struct {
	Jobs []restJob `json:"jobs"`
}
