package github

import (
	"context"
	"net/http"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
)

// UpdatePullRequestBranch merges the base branch of pull request number of
// repo into its head branch, as the "Update branch" button does. A head other
// than "" pins the update to that commit, the head the user saw, so that
// GitHub refuses it if new commits were pushed since.
//
// GitHub starts the merge and answers 202 before it is done, so the head
// moves a moment later. When it refuses, as it does for a branch that is up
// to date, for a conflict, or for a head that moved, the error is a
// *core.RefusedError with GitHub's reason.
//
// It uses REST rather than GraphQL's updatePullRequestBranch: both pin the
// head, but REST answers a refusal with a status, which the other refusals of
// the client are told by, and it needs no node ID.
func (c *Client) UpdatePullRequestBranch(ctx context.Context, repo core.RepoRef, number int, head string) error {
	var body any
	if head != "" {
		body = struct {
			ExpectedHeadSHA string `json:"expected_head_sha"`
		}{head}
	}
	path := commitsBase(repo) + "/pulls/" + strconv.Itoa(number) + "/update-branch"
	return c.refusableDo(ctx, http.MethodPut, path, body, "branch can't be updated")
}
