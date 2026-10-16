package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// pullCommitsQuery reads a page of the commits of a pull request, each with
// the state of its checks, and, on the page that asks for it, the last review
// the viewer submitted. Every field of it is in the oldest Enterprise Server's
// schema. The rollup is null for a commit with no checks.
//
// Its cost is the requests it makes, a hundredth of a point each, rounded to
// the nearest whole point, with at least one: the pull request, its connection
// of commits and one request for the rollup of each commit, so at most 103,
// which is 1 point, for any page.
const pullCommitsQuery = `query PullCommits($owner: String!, $name: String!, $number: Int!, $first: Int!, $after: String, $withReview: Boolean!, $viewer: String!) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      commits(first: $first, after: $after) {
        totalCount
        pageInfo { hasNextPage endCursor }
        nodes {
          commit {
            oid messageHeadline committedDate
            author { name user { login } }
            statusCheckRollup { state }
          }
        }
      }
      reviews(author: $viewer, last: 1, states: [APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED]) @include(if: $withReview) {
        nodes { commit { oid } submittedAt }
      }
    }
  }
}`

// pullCommitNode is the JSON shape of a commit in pullCommitsQuery.
type pullCommitNode struct {
	Commit struct {
		OID             string    `json:"oid"`
		MessageHeadline string    `json:"messageHeadline"`
		CommittedDate   time.Time `json:"committedDate"`
		Author          *struct {
			Name string `json:"name"`
			User *struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"author"`
		StatusCheckRollup *struct {
			State string `json:"state"`
		} `json:"statusCheckRollup"`
	} `json:"commit"`
}

func (n pullCommitNode) core() core.PullCommit {
	c := n.Commit
	out := core.PullCommit{OID: c.OID, Headline: oneLine(c.MessageHeadline), CommittedAt: c.CommittedDate}
	if c.Author != nil {
		out.Author = oneLine(c.Author.Name)
		if c.Author.User != nil {
			out.Login = c.Author.User.Login
		}
	}
	// A rollup that is there but whose state is not one GitHub documents is
	// pending, as CheckStateOf reads it: it never counts as a pass.
	if c.StatusCheckRollup != nil {
		out.Checks = core.CheckStateOf(strings.ToLower(c.StatusCheckRollup.State))
	}
	return out
}

// ListPullCommits returns the commits of pull request number of repo, oldest
// first, each with the state of its checks, and the last review that the user
// viewer submitted on it, which it reads with the first page. An empty viewer
// reads no review. It reads pages of pullCommitsPerPage until it has
// core.MaxPullCommits, which GitHub lists at most; Capped says that the pull
// request has more.
func (c *Client) ListPullCommits(ctx context.Context, repo core.RepoRef, number int, viewer string) (core.PullCommits, error) {
	var (
		out   core.PullCommits
		after string
		total int
	)
	for page := 0; ; page++ {
		var data struct {
			Repository *struct {
				PullRequest *struct {
					Commits struct {
						TotalCount int              `json:"totalCount"`
						PageInfo   pageInfo         `json:"pageInfo"`
						Nodes      []pullCommitNode `json:"nodes"`
					} `json:"commits"`
					Reviews struct {
						Nodes []struct {
							Commit *struct {
								OID string `json:"oid"`
							} `json:"commit"`
							SubmittedAt time.Time `json:"submittedAt"`
						} `json:"nodes"`
					} `json:"reviews"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		vars := map[string]any{
			"owner": repo.Owner, "name": repo.Name, "number": number,
			"first": pullCommitsPerPage, "after": nil,
			"withReview": page == 0 && viewer != "", "viewer": viewer,
		}
		if after != "" {
			vars["after"] = after
		}
		err := c.Query(ctx, pullCommitsQuery, vars, &data)
		if err == nil && (data.Repository == nil || data.Repository.PullRequest == nil) {
			err = core.ErrNotFound
		}
		if err != nil {
			return core.PullCommits{}, fmt.Errorf("list commits of pull %s#%d: %w", repo, number, err)
		}
		pr := data.Repository.PullRequest
		total = pr.Commits.TotalCount
		out.Commits = append(out.Commits, convert(pr.Commits.Nodes, pullCommitNode.core)...)
		if page == 0 && len(pr.Reviews.Nodes) > 0 {
			r := pr.Reviews.Nodes[0]
			out.LastReview = &core.ViewerReview{SubmittedAt: r.SubmittedAt}
			if r.Commit != nil {
				out.LastReview.Commit = r.Commit.OID
			}
		}
		next := pr.Commits.PageInfo.next()
		// A cursor that comes again would loop for ever.
		if next == "" || next == after || len(out.Commits) >= core.MaxPullCommits {
			break
		}
		after = next
	}
	if len(out.Commits) > core.MaxPullCommits {
		out.Commits = out.Commits[:core.MaxPullCommits]
	}
	out.Capped = total > core.MaxPullCommits || total > len(out.Commits)
	return out, nil
}
