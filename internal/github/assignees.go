package github

import (
	"context"
	"net/http"

	"github.com/eggzec/gh-tui/internal/core"
)

// assigneesBody is the body of the calls that add or remove assignees.
type assigneesBody struct {
	Assignees []string `json:"assignees"`
}

// AddIssueAssignees assigns logins to an issue and returns the issue as GitHub
// left it. GitHub silently skips a login it may not assign, one that is not a
// collaborator, so the returned issue is the truth and not the logins asked
// for. A pull request is an issue, so its number works too.
func (c *Client) AddIssueAssignees(ctx context.Context, repo core.RepoRef, number int, logins []string) (core.Issue, error) {
	var it restIssue
	if _, err := c.Do(ctx, http.MethodPost, issuePath(repo, number)+"/assignees", assigneesBody{logins}, &it); err != nil {
		return core.Issue{}, err
	}
	return it.core(repo), nil
}

// RemoveIssueAssignees takes logins off an issue and returns the issue as
// GitHub left it. A login the issue doesn't have is ignored. A pull request
// is an issue, so its number works too.
func (c *Client) RemoveIssueAssignees(ctx context.Context, repo core.RepoRef, number int, logins []string) (core.Issue, error) {
	var it restIssue
	if _, err := c.Do(ctx, http.MethodDelete, issuePath(repo, number)+"/assignees", assigneesBody{logins}, &it); err != nil {
		return core.Issue{}, err
	}
	return it.core(repo), nil
}
