package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// The most that one read of a repository's pinned issues asks for. The
// schema sets no cap, so the read takes this many and keeps that many.
const (
	pinnedIssuesFirst  = 10
	pinnedLabelsFirst  = 20
	pinnedAssigneesMax = 10
)

// pinnedIssuesQuery reads the pinned issues of a repository with the fields
// of an issue that the list shows, in the order they are pinned. The pinned
// issues of a repository exist on github.com and on every Enterprise Server
// the client supports.
var pinnedIssuesQuery = fmt.Sprintf(`query PinnedIssues($owner: String!, $name: String!) {
  `+rateLimitField+`
  repository(owner: $owner, name: $name) {
    pinnedIssues(first: %d) {
      nodes {
        issue {
          id number title body state stateReason url createdAt updatedAt locked activeLockReason
          author { __typename login }
          labels(first: %d) { nodes { name color description } }
          assignees(first: %d) { nodes { login name } }
          comments { totalCount }
        }
      }
    }
  }
}`, pinnedIssuesFirst, pinnedLabelsFirst, pinnedAssigneesMax)

// pinnedIssue is an issue as pinnedIssuesQuery reads it. GraphQL names the
// state and the reasons in capitals, where REST uses lower case.
type pinnedIssue struct {
	ID               string       `json:"id"`
	Number           int          `json:"number"`
	Title            string       `json:"title"`
	Body             string       `json:"body"`
	State            string       `json:"state"`
	StateReason      string       `json:"stateReason"`
	URL              string       `json:"url"`
	CreatedAt        time.Time    `json:"createdAt"`
	UpdatedAt        time.Time    `json:"updatedAt"`
	Locked           bool         `json:"locked"`
	ActiveLockReason string       `json:"activeLockReason"`
	Author           *actor       `json:"author"`
	Labels           nodes[label] `json:"labels"`
	Assignees        nodes[user]  `json:"assignees"`
	CommentCount     struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
}

func (i pinnedIssue) core(repo core.RepoRef) core.Issue {
	out := core.Issue{
		ID:         i.ID,
		Repo:       repo,
		Number:     i.Number,
		Title:      i.Title,
		Body:       i.Body,
		State:      refStates[i.State],
		Reason:     refReasons[i.StateReason],
		Labels:     convert(i.Labels.Nodes, label.core),
		Assignees:  convert(i.Assignees.Nodes, user.core),
		Comments:   i.CommentCount.TotalCount,
		CreatedAt:  i.CreatedAt,
		UpdatedAt:  i.UpdatedAt,
		URL:        i.URL,
		Locked:     i.Locked,
		LockReason: strings.ToLower(i.ActiveLockReason),
	}
	if i.Author != nil {
		out.Author = i.Author.core()
	}
	return out
}

// PinnedIssues reads the issues pinned to repo, in the order they were
// pinned. A repository that pins none, or that GitHub doesn't return, has
// none, and no error. The errors name no read, since the service that calls
// this does.
func (c *Client) PinnedIssues(ctx context.Context, repo core.RepoRef) ([]core.Issue, error) {
	var data struct {
		Repository *struct {
			PinnedIssues nodes[struct {
				Issue *pinnedIssue `json:"issue"`
			}] `json:"pinnedIssues"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name}
	if err := c.Query(ctx, pinnedIssuesQuery, vars, &data); err != nil {
		return nil, err
	}
	if data.Repository == nil {
		return []core.Issue{}, nil
	}
	out := make([]core.Issue, 0, len(data.Repository.PinnedIssues.Nodes))
	for _, n := range data.Repository.PinnedIssues.Nodes {
		if n.Issue == nil || n.Issue.Number == 0 {
			continue
		}
		out = append(out, n.Issue.core(repo))
		if len(out) == pinnedIssuesFirst {
			break
		}
	}
	return out, nil
}
