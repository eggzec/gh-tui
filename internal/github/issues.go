package github

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// The issue endpoints are REST rather than GraphQL because REST answers a
// repeated read with a free 304 when nothing changed.
const (
	// issuesPerPage is above GitHub's default of 30 so that one request
	// fills a screen even after pull requests are filtered out.
	issuesPerPage   = 50
	commentsPerPage = 100
)

// restIssue is the REST shape of an issue. The issue endpoints also return pull
// requests, which are the entries with a pull_request key.
type restIssue struct {
	NodeID      string    `json:"node_id"`
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	User        user      `json:"user"`
	Labels      []label   `json:"labels"`
	Assignees   []user    `json:"assignees"`
	Comments    int       `json:"comments"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	HTMLURL     string    `json:"html_url"`
	PullRequest *struct{} `json:"pull_request"`
}

func (i restIssue) core(repo core.RepoRef) core.Issue {
	return core.Issue{
		ID:        i.NodeID,
		Repo:      repo,
		Number:    i.Number,
		Title:     i.Title,
		Body:      i.Body,
		State:     core.State(i.State),
		Author:    i.User.core(),
		Labels:    convert(i.Labels, label.core),
		Assignees: convert(i.Assignees, user.core),
		Comments:  i.Comments,
		CreatedAt: i.CreatedAt,
		UpdatedAt: i.UpdatedAt,
		URL:       i.HTMLURL,
	}
}

// issueComment is the REST shape of an issue comment.
type issueComment struct {
	NodeID    string    `json:"node_id"`
	User      user      `json:"user"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c issueComment) core() core.Comment {
	return core.Comment{
		ID:        c.NodeID,
		Author:    c.User.core(),
		Body:      c.Body,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// ListIssues returns a page of the repository's issues, most recently
// updated first. Cursor is the Next of the previous page, or empty for the
// first. GitHub counts pull requests towards the page size but they are left
// out, so a page may be short, or even empty, and still not be the last.
func (c *Client) ListIssues(ctx context.Context, repo core.RepoRef, state core.StateFilter, cursor string, cond Conditional) (core.Page[core.Issue], Response, error) {
	path := cursor
	if path == "" {
		q := url.Values{
			"sort":      {"updated"},
			"direction": {"desc"},
			"per_page":  {strconv.Itoa(issuesPerPage)},
		}
		if state != "" {
			q.Set("state", string(state))
		}
		path = issuesPath(repo) + "?" + q.Encode()
	}
	var items []restIssue
	res, err := c.Get(ctx, path, cond, &items)
	if err != nil || res.NotModified {
		return core.Page[core.Issue]{}, res, err
	}
	page := core.Page[core.Issue]{Next: res.Next}
	for i := range items {
		if items[i].PullRequest == nil {
			page.Items = append(page.Items, items[i].core(repo))
		}
	}
	return page, res, nil
}

// GetIssue returns one issue. A pull request number returns the pull
// request's issue part, as GitHub does.
func (c *Client) GetIssue(ctx context.Context, repo core.RepoRef, number int, cond Conditional) (core.Issue, Response, error) {
	var it restIssue
	res, err := c.Get(ctx, issuePath(repo, number), cond, &it)
	if err != nil || res.NotModified {
		return core.Issue{}, res, err
	}
	return it.core(repo), res, nil
}

// ListIssueComments returns a page of the comments on an issue, oldest
// first. Cursor works as in ListIssues.
func (c *Client) ListIssueComments(ctx context.Context, repo core.RepoRef, number int, cursor string, cond Conditional) (core.Page[core.Comment], Response, error) {
	path := cursor
	if path == "" {
		path = issuePath(repo, number) + "/comments?per_page=" + strconv.Itoa(commentsPerPage)
	}
	var items []issueComment
	res, err := c.Get(ctx, path, cond, &items)
	if err != nil || res.NotModified {
		return core.Page[core.Comment]{}, res, err
	}
	return core.Page[core.Comment]{Items: convert(items, issueComment.core), Next: res.Next}, res, nil
}

func issuesPath(repo core.RepoRef) string {
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) + "/issues"
}

func issuePath(repo core.RepoRef, number int) string {
	return issuesPath(repo) + "/" + strconv.Itoa(number)
}
