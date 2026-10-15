package github

import (
	"context"
	"fmt"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
)

// pullCommitsPerPage is how many commits a page of a pull request lists: the
// most GitHub gives.
const pullCommitsPerPage = 100

// ListPullRequestCommits returns the commits of pull request number of repo,
// oldest first, as GitHub lists them, reading as many pages as it takes. If
// cond is current, the Response has NotModified set and the page is empty.
// The Response is that of the first page, whose ETag stands for the whole
// list: GitHub's validators don't hold for the pages after it, so only the
// first is read conditionally.
//
// GitHub lists core.MaxPullCommits commits at most. If the list reaches that
// many, the page is Truncated. GitHub doesn't say how many commits a pull
// request has, so one of exactly that many reads as truncated too. The page
// has no Next: it holds them all.
func (c *Client) ListPullRequestCommits(ctx context.Context, repo core.RepoRef, number int, cond Conditional) (core.Page[core.Commit], Response, error) {
	path := commitsBase(repo) + "/pulls/" + strconv.Itoa(number) + "/commits?per_page=" + strconv.Itoa(pullCommitsPerPage)
	var (
		items []restCommit
		first Response
	)
	for page := 0; path != ""; page++ {
		var got []restCommit
		res, err := c.Get(ctx, path, cond, &got)
		if err != nil {
			return core.Page[core.Commit]{}, first, fmt.Errorf("list commits of pull %s#%d: %w", repo, number, err)
		}
		if page == 0 {
			first = res
			if res.NotModified {
				return core.Page[core.Commit]{}, res, nil
			}
		}
		items = append(items, got...)
		if len(items) >= core.MaxPullCommits {
			break
		}
		path, cond = res.Next, Conditional{}
	}
	truncated := len(items) >= core.MaxPullCommits
	if truncated {
		items = items[:core.MaxPullCommits]
	}
	return core.Page[core.Commit]{Items: convert(items, restCommit.core), Truncated: truncated}, first, nil
}
