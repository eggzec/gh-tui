package github

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
)

// GetIssueKind reports whether number of repo is an issue or a pull
// request, from the issue endpoint, which answers for both. For an issue it
// also returns the issue, as GetIssue would from the same response, so the
// caller may cache it as such. A pull request's issue part isn't returned,
// since it isn't the pull request. A 304 returns an unknown kind.
func (c *Client) GetIssueKind(ctx context.Context, repo core.RepoRef, number int, cond Conditional) (core.NumberKind, core.Issue, Response, error) {
	var it restIssue
	res, err := c.Get(ctx, issuePath(repo, number), cond, &it)
	if err != nil || res.NotModified {
		return "", core.Issue{}, res, err
	}
	if it.PullRequest != nil {
		return core.KindPull, core.Issue{}, res, nil
	}
	return core.KindIssue, it.core(repo), res, nil
}
