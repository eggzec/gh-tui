package github

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// The checks of a commit are read with GraphQL: its status check rollup
// holds the check runs and the commit statuses in one list, with the IDs
// that lead to their jobs and runs, where REST takes a request for each.
// Annotations have no GraphQL equivalent that pages, so they are REST.

// checksContexts is how many checks and statuses a read lists at most.
const checksContexts = 100

// checksFragment selects the rollup of a commit. Required reports whether
// the pull request of $number needs each check, which only a query of a
// pull request can ask.
func checksFragment(required string) string {
	return fmt.Sprintf(`fragment commitChecks on Commit {
  oid
  statusCheckRollup {
    state
    contexts(first: %d) {
      totalCount
      nodes {
        __typename
        ... on CheckRun {
          databaseId name status conclusion detailsUrl startedAt completedAt title summary text
          annotations { totalCount }
          checkSuite { workflowRun { databaseId workflow { name } } }
          %[2]s
        }
        ... on StatusContext { context state description targetUrl createdAt %[2]s }
      }
    }
  }
}`, checksContexts, required)
}

var pullChecksQuery = `query PullChecks($owner: String!, $name: String!, $number: Int!) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      commits(last: 1) { nodes { commit { ...commitChecks } } }
    }
  }
}
` + checksFragment("isRequired(pullRequestNumber: $number)")

var commitChecksQuery = `query CommitChecks($owner: String!, $name: String!, $sha: GitObjectID!) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    object(oid: $sha) { ... on Commit { ...commitChecks } }
  }
}
` + checksFragment("")

// commitChecks is the JSON shape of checksFragment.
type commitChecks struct {
	OID               string `json:"oid"`
	StatusCheckRollup *struct {
		State    string `json:"state"`
		Contexts struct {
			TotalCount int             `json:"totalCount"`
			Nodes      []checksContext `json:"nodes"`
		} `json:"contexts"`
	} `json:"statusCheckRollup"`
}

func (c commitChecks) core() core.Checks {
	out := core.Checks{SHA: c.OID}
	r := c.StatusCheckRollup
	if r == nil {
		return out
	}
	out.State = checksState(r.State)
	out.Total = r.Contexts.TotalCount
	out.Truncated = r.Contexts.TotalCount > len(r.Contexts.Nodes)
	for i := range r.Contexts.Nodes {
		n := &r.Contexts.Nodes[i]
		switch n.Typename {
		case "CheckRun":
			out.Runs = append(out.Runs, n.checkRun())
		case "StatusContext":
			out.Statuses = append(out.Statuses, n.statusContext())
		}
	}
	return out
}

// checksContext is a CheckRun or a StatusContext of a rollup, with what
// core.Checks holds of them.
type checksContext struct {
	Typename   string `json:"__typename"`
	IsRequired bool   `json:"isRequired"`

	// CheckRun fields.
	DatabaseID  int64      `json:"databaseId"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	DetailsURL  string     `json:"detailsUrl"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	Text        string     `json:"text"`
	Annotations struct {
		TotalCount int `json:"totalCount"`
	} `json:"annotations"`
	CheckSuite *struct {
		WorkflowRun *struct {
			DatabaseID int64 `json:"databaseId"`
			Workflow   struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`

	// StatusContext fields.
	Context     string    `json:"context"`
	State       string    `json:"state"`
	Description string    `json:"description"`
	TargetURL   string    `json:"targetUrl"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (c *checksContext) checkRun() core.Check {
	run := core.Check{
		ID:          c.DatabaseID,
		Name:        c.Name,
		Status:      core.RunStatus(strings.ToLower(c.Status)),
		Conclusion:  core.Conclusion(strings.ToLower(c.Conclusion)),
		DetailsURL:  c.DetailsURL,
		Title:       c.Title,
		Summary:     c.Summary,
		Text:        c.Text,
		Annotations: c.Annotations.TotalCount,
		StartedAt:   jobTime(c.StartedAt),
		CompletedAt: jobTime(c.CompletedAt),
		Required:    c.IsRequired,
	}
	// The check run of a job of GitHub Actions has the job's ID.
	if s := c.CheckSuite; s != nil && s.WorkflowRun != nil {
		run.JobID = c.DatabaseID
		run.RunID = s.WorkflowRun.DatabaseID
		run.Workflow = s.WorkflowRun.Workflow.Name
	}
	return run
}

func (c *checksContext) statusContext() core.StatusContext {
	return core.StatusContext{
		Context:     c.Context,
		State:       strings.ToLower(c.State),
		Description: c.Description,
		TargetURL:   c.TargetURL,
		CreatedAt:   c.CreatedAt,
		Required:    c.IsRequired,
	}
}

// PullChecks returns the checks of the head commit of pull request number
// of repo, with whether the pull request requires each. A pull request
// without commits has no checks.
func (c *Client) PullChecks(ctx context.Context, repo core.RepoRef, number int) (core.Checks, error) {
	var data struct {
		Repository *struct {
			PullRequest *struct {
				Commits nodes[struct {
					Commit commitChecks `json:"commit"`
				}] `json:"commits"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	if err := c.Query(ctx, pullChecksQuery, vars, &data); err != nil {
		return core.Checks{}, fmt.Errorf("checks of pull %s#%d: %w", repo, number, err)
	}
	if data.Repository == nil || data.Repository.PullRequest == nil {
		return core.Checks{}, fmt.Errorf("checks of pull %s#%d: %w", repo, number, core.ErrNotFound)
	}
	commits := data.Repository.PullRequest.Commits.Nodes
	if len(commits) == 0 {
		return core.Checks{}, nil
	}
	return commits[0].Commit.core(), nil
}

// CommitChecks returns the checks of commit sha of repo.
func (c *Client) CommitChecks(ctx context.Context, repo core.RepoRef, sha string) (core.Checks, error) {
	var data struct {
		Repository *struct {
			Object *commitChecks `json:"object"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "sha": sha}
	if err := c.Query(ctx, commitChecksQuery, vars, &data); err != nil {
		return core.Checks{}, fmt.Errorf("checks of %s@%s: %w", repo, sha, err)
	}
	if data.Repository == nil || data.Repository.Object == nil || data.Repository.Object.OID == "" {
		return core.Checks{}, fmt.Errorf("checks of %s@%s: %w", repo, sha, core.ErrNotFound)
	}
	return data.Repository.Object.core(), nil
}

// checkAnnotation is the REST shape of an annotation of a check run.
type checkAnnotation struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	StartColumn     *int   `json:"start_column"`
	EndColumn       *int   `json:"end_column"`
	AnnotationLevel string `json:"annotation_level"`
	Title           string `json:"title"`
	Message         string `json:"message"`
	RawDetails      string `json:"raw_details"`
}

func (a checkAnnotation) core() core.Annotation {
	out := core.Annotation{
		Path:       a.Path,
		StartLine:  a.StartLine,
		EndLine:    a.EndLine,
		Level:      core.AnnotationLevel(a.AnnotationLevel),
		Title:      a.Title,
		Message:    a.Message,
		RawDetails: a.RawDetails,
	}
	if a.StartColumn != nil {
		out.StartColumn = *a.StartColumn
	}
	if a.EndColumn != nil {
		out.EndColumn = *a.EndColumn
	}
	return out
}

// ListAnnotations returns a page of the annotations of check run
// checkRunID of repo, which is the ID of its job for GitHub Actions.
// Cursor and perPage work as in ListRuns. If cond is current, the
// Response has NotModified set and the page is empty.
func (c *Client) ListAnnotations(ctx context.Context, repo core.RepoRef, checkRunID int64, cursor string, perPage int, cond Conditional) (core.Page[core.Annotation], Response, error) {
	path := cursor
	if path == "" {
		path = "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) +
			"/check-runs/" + strconv.FormatInt(checkRunID, 10) + "/annotations" + perPageQuery(perPage)
	}
	var items []checkAnnotation
	res, err := c.Get(ctx, path, cond, &items)
	switch {
	case err != nil:
		return core.Page[core.Annotation]{}, res, fmt.Errorf("list annotations of check run %d of %s: %w", checkRunID, repo, err)
	case res.NotModified:
		return core.Page[core.Annotation]{}, res, nil
	}
	return core.Page[core.Annotation]{Items: convert(items, checkAnnotation.core), Next: res.Next}, res, nil
}
