package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// Pull requests are read with GraphQL: REST has neither the review decision
// nor the checks rollup, and one query fetches the labels, author and checks
// that a list row shows. They are changed with GraphQL too, because every
// mutation there returns the updated pull request, where the REST merge
// returns only a commit SHA and REST has no draft endpoints.

// Limits of the nested connections. Rows show a few labels, and the detail
// view shows the latest activity.
const (
	pullLabels    = 20
	pullAssignees = 10
	pullReviews   = 50
	pullComments  = 50
	pullChecks    = 100
)

// pullFields selects what core.PullRequest holds, apart from the body.
var pullFields = fmt.Sprintf(`fragment pullFields on PullRequest {
  id
  number
  title
  state
  isDraft
  url
  createdAt
  updatedAt
  mergedAt
  repository { name owner { login } }
  author { login ... on User { name } }
  labels(first: %d) { nodes { name color description } }
  assignees(first: %d) { nodes { login name } }
  comments { totalCount }
  headRefName
  baseRefName
  reviewDecision
  additions
  deletions
  changedFiles
  commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
}`, pullLabels, pullAssignees)

var listPullsQuery = `query($owner: String!, $name: String!, $states: [PullRequestState!], $first: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: $states, first: $first, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes { ...pullFields }
      pageInfo { hasNextPage endCursor }
    }
  }
}
` + pullFields

var getPullQuery = fmt.Sprintf(`query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      ...pullFields
      body
      reviews(last: %d) {
        nodes { id author { login ... on User { name } } state body submittedAt }
      }
      recentComments: comments(last: %d) {
        nodes { id author { login ... on User { name } } body createdAt updatedAt }
      }
      headCommit: commits(last: 1) {
        nodes { commit { statusCheckRollup { contexts(first: %d) { nodes {
          __typename
          ... on CheckRun { name status conclusion detailsUrl }
          ... on StatusContext { context state targetUrl }
        } } } } }
      }
    }
  }
}
`, pullReviews, pullComments, pullChecks) + pullFields

// pull is the JSON shape of pullFields.
type pull struct {
	ID         string    `json:"id"`
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	State      string    `json:"state"`
	IsDraft    bool      `json:"isDraft"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	MergedAt   time.Time `json:"mergedAt"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	// Author is null for deleted accounts.
	Author    *user        `json:"author"`
	Labels    nodes[label] `json:"labels"`
	Assignees nodes[user]  `json:"assignees"`
	Comments  struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
	HeadRefName    string `json:"headRefName"`
	BaseRefName    string `json:"baseRefName"`
	ReviewDecision string `json:"reviewDecision"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	ChangedFiles   int    `json:"changedFiles"`
	Commits        nodes[struct {
		Commit struct {
			StatusCheckRollup *struct {
				State string `json:"state"`
			} `json:"statusCheckRollup"`
		} `json:"commit"`
	}] `json:"commits"`
}

func (p pull) core() core.PullRequest {
	pr := core.PullRequest{
		ID:             p.ID,
		Repo:           core.RepoRef{Owner: p.Repository.Owner.Login, Name: p.Repository.Name},
		Number:         p.Number,
		Title:          p.Title,
		State:          core.State(strings.ToLower(p.State)),
		Labels:         convert(p.Labels.Nodes, label.core),
		Assignees:      convert(p.Assignees.Nodes, user.core),
		Comments:       p.Comments.TotalCount,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
		URL:            p.URL,
		Draft:          p.IsDraft,
		HeadRef:        p.HeadRefName,
		BaseRef:        p.BaseRefName,
		ReviewDecision: core.ReviewDecision(strings.ToLower(p.ReviewDecision)),
		Additions:      p.Additions,
		Deletions:      p.Deletions,
		ChangedFiles:   p.ChangedFiles,
		MergedAt:       p.MergedAt,
	}
	if p.Author != nil {
		pr.Author = p.Author.core()
	}
	if len(p.Commits.Nodes) > 0 {
		if r := p.Commits.Nodes[0].Commit.StatusCheckRollup; r != nil {
			pr.Checks = checksState(r.State)
		}
	}
	return pr
}

// checksState maps a GraphQL StatusState. EXPECTED means a required check
// has not reported yet, so it counts as pending.
func checksState(s string) core.ChecksState {
	switch s {
	case "SUCCESS":
		return core.ChecksSuccess
	case "PENDING", "EXPECTED":
		return core.ChecksPending
	case "FAILURE", "ERROR":
		return core.ChecksFailure
	default:
		return core.ChecksNone
	}
}

// pullDetail is the JSON shape of the pull request in getPullQuery.
type pullDetail struct {
	pull
	Body           string             `json:"body"`
	Reviews        nodes[review]      `json:"reviews"`
	RecentComments nodes[pullComment] `json:"recentComments"`
	HeadCommit     nodes[struct {
		Commit struct {
			StatusCheckRollup *struct {
				Contexts nodes[checkContext] `json:"contexts"`
			} `json:"statusCheckRollup"`
		} `json:"commit"`
	}] `json:"headCommit"`
}

func (d pullDetail) core() core.PullRequestDetail {
	pr := d.pull.core()
	pr.Body = d.Body
	out := core.PullRequestDetail{
		PullRequest:    pr,
		Reviews:        convert(d.Reviews.Nodes, review.core),
		RecentComments: convert(d.RecentComments.Nodes, pullComment.core),
	}
	if len(d.HeadCommit.Nodes) > 0 {
		if r := d.HeadCommit.Nodes[0].Commit.StatusCheckRollup; r != nil {
			out.CheckRuns = convert(r.Contexts.Nodes, checkContext.core)
		}
	}
	return out
}

type review struct {
	ID          string    `json:"id"`
	Author      *user     `json:"author"`
	State       string    `json:"state"`
	Body        string    `json:"body"`
	SubmittedAt time.Time `json:"submittedAt"`
}

func (r review) core() core.Review {
	out := core.Review{
		ID:          r.ID,
		State:       core.ReviewState(strings.ToLower(r.State)),
		Body:        r.Body,
		SubmittedAt: r.SubmittedAt,
	}
	if r.Author != nil {
		out.Author = r.Author.core()
	}
	return out
}

type pullComment struct {
	ID        string    `json:"id"`
	Author    *user     `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (c pullComment) core() core.Comment {
	out := core.Comment{ID: c.ID, Body: c.Body, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
	if c.Author != nil {
		out.Author = c.Author.core()
	}
	return out
}

// checkContext is a CheckRun or a StatusContext, the two kinds of checks in a
// rollup.
type checkContext struct {
	Typename string `json:"__typename"`
	// CheckRun fields.
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	DetailsURL string `json:"detailsUrl"`
	// StatusContext fields.
	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"targetUrl"`
}

func (c checkContext) core() core.CheckRun {
	if c.Typename == "StatusContext" {
		run := core.CheckRun{Name: c.Context, Status: "completed", Conclusion: strings.ToLower(c.State), URL: c.TargetURL}
		if checksState(c.State) == core.ChecksPending {
			run.Status, run.Conclusion = "pending", ""
		}
		return run
	}
	return core.CheckRun{
		Name:       c.Name,
		Status:     strings.ToLower(c.Status),
		Conclusion: strings.ToLower(c.Conclusion),
		URL:        c.DetailsURL,
	}
}

// pullStates maps a state filter to the GraphQL PullRequestState list. The
// empty state selects every pull request.
func pullStates(s core.State) ([]string, error) {
	switch s {
	case "":
		return nil, nil
	case core.StateOpen, core.StateClosed, core.StateMerged:
		return []string{strings.ToUpper(string(s))}, nil
	default:
		return nil, fmt.Errorf("unknown pull request state %q", s)
	}
}

// ListPullRequests returns a page of up to first pull requests of repo in
// state, most recently updated first. An empty state lists them all. Cursor
// is the Next of the previous page, or empty for the first page. GitHub
// accepts a first of 1 to 100. The results have no Body.
func (c *Client) ListPullRequests(ctx context.Context, repo core.RepoRef, state core.State, cursor string, first int) (core.Page[core.PullRequest], error) {
	states, err := pullStates(state)
	if err != nil {
		return core.Page[core.PullRequest]{}, fmt.Errorf("list pull requests of %s: %w", repo, err)
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "states": states, "first": first}
	if cursor != "" {
		vars["after"] = cursor
	}
	var data struct {
		Repository *struct {
			PullRequests struct {
				Nodes    []pull   `json:"nodes"`
				PageInfo pageInfo `json:"pageInfo"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}
	if err := c.Query(ctx, listPullsQuery, vars, &data); err != nil {
		return core.Page[core.PullRequest]{}, fmt.Errorf("list pull requests of %s: %w", repo, err)
	}
	if data.Repository == nil {
		return core.Page[core.PullRequest]{}, fmt.Errorf("list pull requests of %s: %w", repo, core.ErrNotFound)
	}
	conn := data.Repository.PullRequests
	return core.Page[core.PullRequest]{
		Items: convert(conn.Nodes, pull.core),
		Next:  conn.PageInfo.next(),
	}, nil
}

// GetPullRequest returns pull request number of repo with its body, latest
// reviews and comments, and the checks of its head commit.
func (c *Client) GetPullRequest(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error) {
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	var data struct {
		Repository *struct {
			PullRequest *pullDetail `json:"pullRequest"`
		} `json:"repository"`
	}
	err := c.Query(ctx, getPullQuery, vars, &data)
	if err == nil && (data.Repository == nil || data.Repository.PullRequest == nil) {
		err = core.ErrNotFound
	}
	if err != nil {
		return core.PullRequestDetail{}, fmt.Errorf("get pull request %s#%d: %w", repo, number, err)
	}
	return data.Repository.PullRequest.core(), nil
}

var pullIDQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) { pullRequest(number: $number) { id } }
}`

// PullRequestID returns the node ID of pull request number of repo, which
// the mutations take.
func (c *Client) PullRequestID(ctx context.Context, repo core.RepoRef, number int) (string, error) {
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	var data struct {
		Repository *struct {
			PullRequest *struct {
				ID string `json:"id"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	err := c.Query(ctx, pullIDQuery, vars, &data)
	if err == nil && (data.Repository == nil || data.Repository.PullRequest == nil) {
		err = core.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("look up pull request %s#%d: %w", repo, number, err)
	}
	return data.Repository.PullRequest.ID, nil
}

// pullMutation is a mutation on the pull request with node ID $id. Its
// payload is aliased to result, so that one shape decodes them all.
func pullMutation(field string) string {
	return fmt.Sprintf(`mutation($id: ID!) {
  result: %s(input: {pullRequestId: $id}) { pullRequest { ...pullFields } }
}
`, field) + pullFields
}

var (
	mergePullMutation = `mutation($id: ID!, $method: PullRequestMergeMethod!) {
  result: mergePullRequest(input: {pullRequestId: $id, mergeMethod: $method}) { pullRequest { ...pullFields } }
}
` + pullFields
	closePullMutation   = pullMutation("closePullRequest")
	reopenPullMutation  = pullMutation("reopenPullRequest")
	readyPullMutation   = pullMutation("markPullRequestReadyForReview")
	toDraftPullMutation = pullMutation("convertPullRequestToDraft")
)

// errNoPull is returned when a mutation succeeds without returning the pull
// request, which GitHub should not do.
var errNoPull = errors.New("response has no pull request")

// mutatePull runs a pull request mutation and returns the updated pull
// request. What names the mutation in errors.
func (c *Client) mutatePull(ctx context.Context, what, query string, vars map[string]any) (core.PullRequest, error) {
	var data struct {
		Result *struct {
			PullRequest *pull `json:"pullRequest"`
		} `json:"result"`
	}
	err := c.Query(ctx, query, vars, &data)
	if err == nil && (data.Result == nil || data.Result.PullRequest == nil) {
		err = errNoPull
	}
	if err != nil {
		return core.PullRequest{}, fmt.Errorf("%s pull request %s: %w", what, vars["id"], err)
	}
	return data.Result.PullRequest.core(), nil
}

// MergePullRequest merges the pull request with node ID id using method and
// returns it as merged.
func (c *Client) MergePullRequest(ctx context.Context, id string, method core.MergeMethod) (core.PullRequest, error) {
	switch method {
	case core.MergeCommit, core.MergeSquash, core.MergeRebase:
	default:
		return core.PullRequest{}, fmt.Errorf("merge pull request %s: unknown merge method %q", id, method)
	}
	vars := map[string]any{"id": id, "method": strings.ToUpper(string(method))}
	return c.mutatePull(ctx, "merge", mergePullMutation, vars)
}

// ClosePullRequest closes the pull request with node ID id without merging
// it.
func (c *Client) ClosePullRequest(ctx context.Context, id string) (core.PullRequest, error) {
	return c.mutatePull(ctx, "close", closePullMutation, map[string]any{"id": id})
}

// ReopenPullRequest reopens the closed pull request with node ID id.
func (c *Client) ReopenPullRequest(ctx context.Context, id string) (core.PullRequest, error) {
	return c.mutatePull(ctx, "reopen", reopenPullMutation, map[string]any{"id": id})
}

// MarkPullRequestReady marks the draft pull request with node ID id as ready
// for review.
func (c *Client) MarkPullRequestReady(ctx context.Context, id string) (core.PullRequest, error) {
	return c.mutatePull(ctx, "mark ready", readyPullMutation, map[string]any{"id": id})
}

// ConvertPullRequestToDraft turns the pull request with node ID id back into
// a draft.
func (c *Client) ConvertPullRequestToDraft(ctx context.Context, id string) (core.PullRequest, error) {
	return c.mutatePull(ctx, "convert to draft", toDraftPullMutation, map[string]any{"id": id})
}
