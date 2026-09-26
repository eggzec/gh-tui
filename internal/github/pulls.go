package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// Pull requests are read with GraphQL: REST has neither the review decision
// nor the checks rollup, and one query fetches the labels, author and checks
// that a list row shows. They are changed with GraphQL too, because every
// mutation there returns the updated pull request, where the REST merge
// returns only a commit SHA and REST has no draft endpoints. Their comments
// are those of their issue, which REST reads with a free 304 when nothing
// changed: see ListIssueComments.

// Limits of the nested connections. Rows show a few labels. Reviews and
// comments are paged on their own, as a thread can be long.
const (
	pullLabels    = 20
	pullAssignees = 10
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
  locked
  activeLockReason
  viewerCanUpdate
  viewerCanClose
  viewerCanReopen
  viewerCanLabel
  viewerDidAuthor
}`, pullLabels, pullAssignees)

var listPullsQuery = `query ListPulls($owner: String!, $name: String!, $states: [PullRequestState!], $labels: [String!],
  $base: String, $head: String, $order: IssueOrder!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    pullRequests(states: $states, labels: $labels, baseRefName: $base, headRefName: $head, orderBy: $order, first: $first, after: $after) {
      nodes { ...pullFields }
      pageInfo { hasNextPage endCursor }
    }
  }
}
` + pullFields

// getPullQuery reads what the detail view shows on top of pullFields. The
// checks of the head commit are only counted by outcome: the checks step
// lists them with PullChecks.
var getPullQuery = `query GetPull($owner: String!, $name: String!, $number: Int!) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      ...pullFields
      body
      headCommit: commits(last: 1) {
        nodes { commit { statusCheckRollup { contexts {
          checkRunCountsByState { state count }
          statusContextCountsByState { state count }
        } } } }
      }
    }
  }
}
` + pullFields

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
	viewerCaps
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
	pr.Locked, pr.LockReason, pr.Caps = p.Locked, strings.ToLower(p.ActiveLockReason), p.viewerCaps.core()
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
	Body       string `json:"body"`
	HeadCommit nodes[struct {
		Commit struct {
			StatusCheckRollup *struct {
				Contexts pullCheckCounts `json:"contexts"`
			} `json:"statusCheckRollup"`
		} `json:"commit"`
	}] `json:"headCommit"`
}

func (d pullDetail) core() core.PullRequestDetail {
	pr := d.pull.core()
	pr.Body = d.Body
	out := core.PullRequestDetail{PullRequest: pr}
	if len(d.HeadCommit.Nodes) > 0 {
		if r := d.HeadCommit.Nodes[0].Commit.StatusCheckRollup; r != nil {
			out.CheckCounts = r.Contexts.core()
		}
	}
	return out
}

// pullCheckCounts is how many checks of a rollup are in each state: check
// runs by CheckRunState, which folds their status and conclusion into one,
// and commit statuses by StatusState.
type pullCheckCounts struct {
	CheckRuns []pullStateCount `json:"checkRunCountsByState"`
	Statuses  []pullStateCount `json:"statusContextCountsByState"`
}

type pullStateCount struct {
	State string `json:"state"`
	Count int    `json:"count"`
}

func (c pullCheckCounts) core() core.CheckCounts {
	var out core.CheckCounts
	add := func(s core.ChecksState, n int) {
		switch s {
		case core.ChecksSuccess:
			out.Passed += n
		case core.ChecksPending:
			out.Pending += n
		default:
			out.Failed += n
		}
	}
	for _, r := range c.CheckRuns {
		add(checkRunState(r.State), r.Count)
	}
	for _, st := range c.Statuses {
		add(checksState(st.State), st.Count)
	}
	return out
}

// checkRunState maps a CheckRunState. A run that hasn't completed is
// pending, one that was neutral or skipped passed, and one that completed
// any other way, even without a conclusion, failed.
func checkRunState(s string) core.ChecksState {
	switch s {
	case "QUEUED", "IN_PROGRESS", "PENDING", "WAITING":
		return core.ChecksPending
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return core.ChecksSuccess
	default:
		return core.ChecksFailure
	}
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
	return c.FilterPullRequests(ctx, repo, PullFilter{State: state}, cursor, first)
}

// PullFilter selects the pull requests of a repository that its list can
// select on the server. The zero value lists them all, most recently
// updated first. Anything else takes a search: see SearchPullRequests.
type PullFilter struct {
	// State is open, closed (and not merged) or merged, or empty for all.
	State core.State
	// Labels selects the pull requests with any of them.
	Labels []string
	// Base and Head select by the name of the branch merged into, and of
	// the branch merged.
	Base, Head string
	// Sort is updated, created or comments; empty means updated. Asc
	// sorts the oldest or least commented first.
	Sort string
	Asc  bool
}

// pullOrder maps the sort of f to a GraphQL IssueOrder.
func pullOrder(f PullFilter) (map[string]string, error) {
	field := ""
	switch f.Sort {
	case "", "updated":
		field = "UPDATED_AT"
	case "created":
		field = "CREATED_AT"
	case "comments":
		field = "COMMENTS"
	default:
		return nil, fmt.Errorf("unknown pull request sort %q", f.Sort)
	}
	dir := "DESC"
	if f.Asc {
		dir = "ASC"
	}
	return map[string]string{"field": field, "direction": dir}, nil
}

// FilterPullRequests returns a page of up to first pull requests of repo
// that f selects, as ListPullRequests does.
func (c *Client) FilterPullRequests(ctx context.Context, repo core.RepoRef, f PullFilter, cursor string, first int) (core.Page[core.PullRequest], error) {
	states, err := pullStates(f.State)
	if err != nil {
		return core.Page[core.PullRequest]{}, fmt.Errorf("list pull requests of %s: %w", repo, err)
	}
	order, err := pullOrder(f)
	if err != nil {
		return core.Page[core.PullRequest]{}, fmt.Errorf("list pull requests of %s: %w", repo, err)
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "states": states, "order": order, "first": first}
	if cursor != "" {
		vars["after"] = cursor
	}
	if len(f.Labels) > 0 {
		vars["labels"] = f.Labels
	}
	if f.Base != "" {
		vars["base"] = f.Base
	}
	if f.Head != "" {
		vars["head"] = f.Head
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

// ProbePullRequests reports whether any pull request of repo changed since
// the response cond came from, as a 304 when none did, which costs no rate
// limit. It is a REST request, as GraphQL has no conditional requests. Only
// the Response is returned: pass its ETag as cond next time.
func (c *Client) ProbePullRequests(ctx context.Context, repo core.RepoRef, cond Conditional) (Response, error) {
	return c.probeList(ctx, "repos/"+url.PathEscape(repo.Owner)+"/"+url.PathEscape(repo.Name)+"/pulls", cond)
}

// GetPullRequest returns pull request number of repo with its body and the
// counts of the checks of its head commit. Its reviews are read a page at a
// time with ListPullRequestReviews, and its comments, which are those of
// its issue, with ListIssueComments.
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

// pullPage is the query op, which selects a page of the connection field
// of a pull request, such as its reviews, oldest first. The page is
// aliased to page, so that one shape decodes them all.
func pullPage(op, field, nodeFields string) string {
	return fmt.Sprintf(`query %s($owner: String!, $name: String!, $number: Int!, $first: Int!, $after: String) {
  %s
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      page: %s(first: $first, after: $after) {
        nodes { %s }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`, op, rateLimitField, field, nodeFields)
}

var pullReviewsQuery = pullPage("PullReviews", "reviews", "id author { login ... on User { name } } state body submittedAt")

// listPullPage runs a pullPage query and converts its nodes with f. What
// names the page in errors.
func listPullPage[T, U any](
	ctx context.Context, c *Client, what, query string,
	repo core.RepoRef, number int, cursor string, first int,
	f func(T) U,
) (core.Page[U], error) {
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number, "first": first}
	if cursor != "" {
		vars["after"] = cursor
	}
	var data struct {
		Repository *struct {
			PullRequest *struct {
				Page struct {
					Nodes    []T      `json:"nodes"`
					PageInfo pageInfo `json:"pageInfo"`
				} `json:"page"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	err := c.Query(ctx, query, vars, &data)
	if err == nil && (data.Repository == nil || data.Repository.PullRequest == nil) {
		err = core.ErrNotFound
	}
	if err != nil {
		return core.Page[U]{}, fmt.Errorf("list %s of pull request %s#%d: %w", what, repo, number, err)
	}
	page := data.Repository.PullRequest.Page
	return core.Page[U]{Items: convert(page.Nodes, f), Next: page.PageInfo.next()}, nil
}

// ListPullRequestReviews returns a page of up to first reviews of pull
// request number of repo, oldest first. Cursor and first work as in
// ListPullRequests.
func (c *Client) ListPullRequestReviews(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.Review], error) {
	return listPullPage(ctx, c, "reviews", pullReviewsQuery, repo, number, cursor, first, review.core)
}

var pullIDQuery = `query PullID($owner: String!, $name: String!, $number: Int!) {
  ` + rateLimitField + `
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
	return fmt.Sprintf(`mutation %s($id: ID!) {
  result: %s(input: {pullRequestId: $id}) { pullRequest { ...pullFields } }
}
`, strings.ToUpper(field[:1])+field[1:], field) + pullFields
}

var (
	mergePullMutation = `mutation MergePullRequest($id: ID!, $method: PullRequestMergeMethod!) {
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
