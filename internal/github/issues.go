package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// The issue endpoints are REST rather than GraphQL because REST answers a
// repeated read with a free 304 when nothing changed.

// restIssue is the REST shape of an issue. The issue endpoints also return pull
// requests, which are the entries with a pull_request key.
type restIssue struct {
	NodeID      string    `json:"node_id"`
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	StateReason string    `json:"state_reason"`
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
		Reason:    core.StateReason(i.StateReason),
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
// first. PerPage sizes the first page, and 0 leaves the size to GitHub; later
// pages keep the size of the page their cursor came from. GitHub counts pull
// requests towards the page size but they are left out, so a page may be
// short, or even empty, and still not be the last.
func (c *Client) ListIssues(ctx context.Context, repo core.RepoRef, state core.StateFilter, cursor string, perPage int, cond Conditional) (core.Page[core.Issue], Response, error) {
	path := cursor
	if path == "" {
		q := url.Values{
			"sort":      {"updated"},
			"direction": {"desc"},
		}
		if perPage > 0 {
			q.Set("per_page", strconv.Itoa(perPage))
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
// first. Cursor and perPage work as in ListIssues.
func (c *Client) ListIssueComments(ctx context.Context, repo core.RepoRef, number int, cursor string, perPage int, cond Conditional) (core.Page[core.Comment], Response, error) {
	path := cursor
	if path == "" {
		path = issuePath(repo, number) + "/comments"
		if perPage > 0 {
			path += "?per_page=" + strconv.Itoa(perPage)
		}
	}
	var items []issueComment
	res, err := c.Get(ctx, path, cond, &items)
	if err != nil || res.NotModified {
		return core.Page[core.Comment]{}, res, err
	}
	return core.Page[core.Comment]{Items: convert(items, issueComment.core), Next: res.Next}, res, nil
}

// ProbeIssues reports whether any issue or pull request of repo changed
// since the response cond came from, as a 304 when none did, which costs no
// rate limit. Only the Response is returned: pass its ETag as cond next time.
// GitHub counts pull requests as issues, so their changes show here too.
func (c *Client) ProbeIssues(ctx context.Context, repo core.RepoRef, cond Conditional) (Response, error) {
	return c.probeList(ctx, issuesPath(repo), cond)
}

func issuesPath(repo core.RepoRef) string {
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) + "/issues"
}

func issuePath(repo core.RepoRef, number int) string {
	return issuesPath(repo) + "/" + strconv.Itoa(number)
}

// SetIssueState closes or reopens an issue and returns the issue as GitHub
// stored it.
func (c *Client) SetIssueState(ctx context.Context, repo core.RepoRef, number int, state core.State) (core.Issue, error) {
	body := struct {
		State core.State `json:"state"`
	}{state}
	var it restIssue
	if _, err := c.Do(ctx, http.MethodPatch, issuePath(repo, number), body, &it); err != nil {
		return core.Issue{}, err
	}
	return it.core(repo), nil
}

// AddIssueLabels adds labels to an issue by name and returns all of the
// issue's labels. GitHub creates names the repository doesn't have yet.
func (c *Client) AddIssueLabels(ctx context.Context, repo core.RepoRef, number int, names []string) ([]core.Label, error) {
	body := struct {
		Labels []string `json:"labels"`
	}{names}
	var labels []label
	if _, err := c.Do(ctx, http.MethodPost, issuePath(repo, number)+"/labels", body, &labels); err != nil {
		return nil, err
	}
	return convert(labels, label.core), nil
}

// RemoveIssueLabel removes a label from an issue and returns the labels
// left. It fails with core.ErrNotFound if the issue doesn't have the label.
func (c *Client) RemoveIssueLabel(ctx context.Context, repo core.RepoRef, number int, name string) ([]core.Label, error) {
	var labels []label
	path := issuePath(repo, number) + "/labels/" + url.PathEscape(name)
	if _, err := c.Do(ctx, http.MethodDelete, path, nil, &labels); err != nil {
		return nil, err
	}
	return convert(labels, label.core), nil
}

// CreateIssueComment comments on an issue and returns the new comment.
func (c *Client) CreateIssueComment(ctx context.Context, repo core.RepoRef, number int, body string) (core.Comment, error) {
	in := struct {
		Body string `json:"body"`
	}{body}
	var out issueComment
	if _, err := c.Do(ctx, http.MethodPost, issuePath(repo, number)+"/comments", in, &out); err != nil {
		return core.Comment{}, err
	}
	return out.core(), nil
}
