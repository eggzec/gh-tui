package github

import (
	"context"
	"fmt"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
)

// pullFilesPerPage is how many files a page of a pull request lists: the
// most GitHub gives.
const pullFilesPerPage = 100

// ListPullRequestFiles returns a page of the files that pull request
// number of repo changes, against the commit its branch forked from, with
// their patches. Cursor is the Next of the previous page; empty reads the
// first. If cond is current, the Response has NotModified set and the page
// is empty.
//
// GitHub lists MaxPullFiles files at most. If the pages reach that many,
// the first page is Truncated. GitHub doesn't say how many files a pull
// request changes, so one of exactly that many reads as truncated too.
func (c *Client) ListPullRequestFiles(ctx context.Context, repo core.RepoRef, number int, cursor string, cond Conditional) (core.Page[core.CommitFile], Response, error) {
	path := cursor
	if path == "" {
		path = commitsBase(repo) + "/pulls/" + strconv.Itoa(number) + "/files?per_page=" + strconv.Itoa(pullFilesPerPage)
	}
	var items []commitFile
	res, err := c.Get(ctx, path, cond, &items)
	switch {
	case err != nil:
		return core.Page[core.CommitFile]{}, res, fmt.Errorf("list files of pull %s#%d: %w", repo, number, err)
	case res.NotModified:
		return core.Page[core.CommitFile]{}, res, nil
	}
	page := core.Page[core.CommitFile]{Items: convert(items, commitFile.core), Next: res.Next}
	// Only the pages before the last say where the last is, so the first
	// reports it, as GetCommit does.
	page.Truncated = cursor == "" && filesTruncated(res)
	return page, res, nil
}
