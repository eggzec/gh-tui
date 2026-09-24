package github

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
)

// restBranch is the REST shape of a branch in a list.
type restBranch struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
	Protected bool `json:"protected"`
}

func (b restBranch) core() core.Branch {
	return core.Branch{Name: b.Name, SHA: b.Commit.SHA, Protected: b.Protected}
}

// ListBranches returns a page of the repository's branches, by name.
// Cursor is the Next of the previous page, or empty for the first. PerPage
// sizes the first page, and 0 leaves the size to GitHub; later pages keep
// the size of the page their cursor came from. If cond is current, the
// Response has NotModified set and the page is empty. An empty repository
// has no branches.
func (c *Client) ListBranches(ctx context.Context, repo core.RepoRef, cursor string, perPage int, cond Conditional) (core.Page[core.Branch], Response, error) {
	path := cursor
	if path == "" {
		path = branchesPath(repo)
		if perPage > 0 {
			path += "?per_page=" + strconv.Itoa(perPage)
		}
	}
	var items []restBranch
	res, err := c.Get(ctx, path, cond, &items)
	if err != nil {
		return core.Page[core.Branch]{}, res, fmt.Errorf("list branches: %w", err)
	}
	if res.NotModified {
		return core.Page[core.Branch]{}, res, nil
	}
	return core.Page[core.Branch]{Items: convert(items, restBranch.core), Next: res.Next}, res, nil
}

func branchesPath(repo core.RepoRef) string {
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) + "/branches"
}
