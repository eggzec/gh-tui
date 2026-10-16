package github

import (
	"context"
	"errors"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
)

// pullViewedPerPage is how many files a page of the viewed marks lists: the
// most GitHub gives.
const pullViewedPerPage = 100

// pullViewedQuery reads a page of the files of a pull request, each with the
// viewer's mark on it.
//
// Its cost is about 1 point a page: the pull request and its connection of
// files, and no node under a file.
const pullViewedQuery = `query PullViewed($owner: String!, $name: String!, $number: Int!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      files(first: $first, after: $after) {
        totalCount
        pageInfo { hasNextPage endCursor }
        nodes { path viewerViewedState }
      }
    }
  }
}`

// pullViewedState reads the state of GitHub's viewed mark, which an answer
// that lacks the field, as one of an old Enterprise Server does, leaves empty.
func pullViewedState(s string) core.ViewedState {
	switch s {
	case "VIEWED":
		return core.Viewed
	case "DISMISSED":
		return core.ViewedChanged
	default:
		return core.Unviewed
	}
}

// ListPullViewed returns the viewer's viewed marks on the files of pull
// request number of repo, as of its head. It reads pages of pullViewedPerPage
// until it has core.MaxPullFiles files, which GitHub lists at most; Capped
// says that the pull request has more.
//
// A server whose schema lacks the mark is read without it (see unsupported.go),
// and the answer is not Supported.
func (c *Client) ListPullViewed(ctx context.Context, repo core.RepoRef, number int) (core.PullViewed, error) {
	var (
		out     = core.PullViewed{States: map[string]core.ViewedState{}}
		after   string
		total   int
		files   int
		stated  bool
		visited = map[string]bool{}
	)
	for {
		var data struct {
			Repository *struct {
				PullRequest *struct {
					Files struct {
						TotalCount int      `json:"totalCount"`
						PageInfo   pageInfo `json:"pageInfo"`
						Nodes      []struct {
							Path              string  `json:"path"`
							ViewerViewedState *string `json:"viewerViewedState"`
						} `json:"nodes"`
					} `json:"files"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		vars := map[string]any{
			"owner": repo.Owner, "name": repo.Name, "number": number,
			"first": pullViewedPerPage, "after": nil,
		}
		if after != "" {
			vars["after"] = after
		}
		err := c.Query(ctx, pullViewedQuery, vars, &data)
		if err == nil && (data.Repository == nil || data.Repository.PullRequest == nil) {
			err = core.ErrNotFound
		}
		if err != nil {
			return core.PullViewed{}, fmt.Errorf("list viewed files of pull %s#%d: %w", repo, number, err)
		}
		pr := data.Repository.PullRequest
		total = pr.Files.TotalCount
		for _, n := range pr.Files.Nodes {
			if files >= core.MaxPullFiles {
				break
			}
			files++
			if n.ViewerViewedState == nil {
				continue
			}
			stated = true
			if st := pullViewedState(*n.ViewerViewedState); st != core.Unviewed {
				out.States[n.Path] = st
			}
		}
		next := pr.Files.PageInfo.next()
		// A cursor that comes again would loop for ever.
		if next == "" || visited[next] || files >= core.MaxPullFiles {
			break
		}
		visited[next] = true
		after = next
	}
	out.Capped = total > core.MaxPullFiles || total > files
	out.Supported = stated || files == 0
	return out, nil
}

// The mutations that mark a file of a pull request viewed and unmark it. The
// payload is aliased to result, so that one shape decodes both.
const (
	markFileViewedMutation = `mutation MarkFileAsViewed($id: ID!, $path: String!) {
  result: markFileAsViewed(input: {pullRequestId: $id, path: $path}) { pullRequest { id } }
}`
	unmarkFileViewedMutation = `mutation UnmarkFileAsViewed($id: ID!, $path: String!) {
  result: unmarkFileAsViewed(input: {pullRequestId: $id, path: $path}) { pullRequest { id } }
}`
)

// SetFileViewed marks the file at path of the pull request with node ID
// pullID viewed, or unmarks it if viewed is false. The mark is the viewer's
// alone, and read access is enough to set it.
func (c *Client) SetFileViewed(ctx context.Context, pullID, path string, viewed bool) error {
	if viewed {
		return c.setFileViewed(ctx, markFileViewedMutation, "mark", pullID, path)
	}
	return c.setFileViewed(ctx, unmarkFileViewedMutation, "unmark", pullID, path)
}

func (c *Client) setFileViewed(ctx context.Context, query, what, pullID, path string) error {
	var data struct {
		Result *struct {
			PullRequest *struct {
				ID string `json:"id"`
			} `json:"pullRequest"`
		} `json:"result"`
	}
	err := c.Query(ctx, query, map[string]any{"id": pullID, "path": path}, &data)
	if err == nil && (data.Result == nil || data.Result.PullRequest == nil) {
		err = errNoViewedPull
	}
	if err != nil {
		return fmt.Errorf("%s file %s as viewed on pull request %s: %w", what, path, pullID, err)
	}
	return nil
}

// errNoViewedPull is returned when a mutation succeeds without returning the
// pull request, which GitHub should not do.
var errNoViewedPull = errors.New("response has no pull request")
