package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/eggzec/gh-tui/internal/core"
)

// What the search preview of a repository reads beyond the hit: its README,
// its latest release and how many issues and pull requests are open.

func repoPreviewPath(repo core.RepoRef) string {
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name)
}

// RepoReadme returns the README of repo as Markdown. A repository without
// one is not an error: the text is empty.
func (c *Client) RepoReadme(ctx context.Context, repo core.RepoRef) (string, error) {
	b, _, err := c.getRaw(ctx, repoPreviewPath(repo)+"/readme", Conditional{}, maxReadme)
	if errors.Is(err, core.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("README of %s: %w", repo, err)
	}
	return string(b), nil
}

// LatestRelease returns the latest published release of repo, and false if it
// has none.
func (c *Client) LatestRelease(ctx context.Context, repo core.RepoRef) (core.Release, bool, error) {
	var r restRelease
	if _, err := c.Get(ctx, repoPreviewPath(repo)+"/releases/latest", Conditional{}, &r); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return core.Release{}, false, nil
		}
		return core.Release{}, false, fmt.Errorf("latest release of %s: %w", repo, err)
	}
	return r.core(), true, nil
}

const openCountsQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    issues(states: OPEN) { totalCount }
    pullRequests(states: OPEN) { totalCount }
  }
}`

// OpenCounts returns how many issues and how many pull requests are open in
// repo.
func (c *Client) OpenCounts(ctx context.Context, repo core.RepoRef) (issues, pulls int, err error) {
	var out struct {
		Repository struct {
			Issues       struct{ TotalCount int } `json:"issues"`
			PullRequests struct{ TotalCount int } `json:"pullRequests"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name}
	if err := c.Query(ctx, openCountsQuery, vars, &out); err != nil {
		return 0, 0, fmt.Errorf("open counts of %s: %w", repo, err)
	}
	return out.Repository.Issues.TotalCount, out.Repository.PullRequests.TotalCount, nil
}
