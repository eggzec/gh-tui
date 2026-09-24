package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// viewerWorkQuery runs the three searches of the work waiting on the viewer
// in one request. GraphQL search counts against the GraphQL quota, not the
// REST search limit of 30 requests a minute, and the three together cost
// one point.
const viewerWorkQuery = `query ViewerWork($first: Int!) {
  ` + rateLimitField + `
  reviewRequested: search(type: ISSUE, first: $first, query: "is:open is:pr review-requested:@me archived:false sort:updated-desc") { ...workHits }
  authored: search(type: ISSUE, first: $first, query: "is:open is:pr author:@me archived:false sort:updated-desc") { ...workHits }
  assigned: search(type: ISSUE, first: $first, query: "is:open is:issue assignee:@me archived:false sort:updated-desc") { ...workHits }
}

fragment workHits on SearchResultItemConnection {
  issueCount
  nodes {
    __typename
    ... on PullRequest { id number title state author { login } comments { totalCount } createdAt updatedAt url repository { name owner { login } } }
    ... on Issue { id number title state author { login } comments { totalCount } createdAt updatedAt url repository { name owner { login } } }
  }
}`

// workNode is an issue or a pull request in a search result.
type workNode struct {
	Typename string `json:"__typename"`
	ID       string `json:"id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	State    string `json:"state"`
	// Author is null for a deleted account.
	Author     *user       `json:"author"`
	Comments   viewerCount `json:"comments"`
	CreatedAt  time.Time   `json:"createdAt"`
	UpdatedAt  time.Time   `json:"updatedAt"`
	URL        string      `json:"url"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

func (n workNode) core() core.SearchHit {
	hit := core.SearchHit{
		Kind: core.SearchIssues,
		Issue: core.Issue{
			ID:        n.ID,
			Repo:      core.RepoRef{Owner: n.Repository.Owner.Login, Name: n.Repository.Name},
			Number:    n.Number,
			Title:     n.Title,
			State:     core.State(strings.ToLower(n.State)),
			Comments:  n.Comments.TotalCount,
			CreatedAt: n.CreatedAt,
			UpdatedAt: n.UpdatedAt,
			URL:       n.URL,
		},
	}
	if n.Typename == "PullRequest" {
		hit.Kind = core.SearchPulls
	}
	if n.Author != nil {
		hit.Issue.Author = n.Author.core()
	}
	return hit
}

type workHits struct {
	IssueCount int `json:"issueCount"`
	nodes[workNode]
}

func (h workHits) core() core.WorkList {
	return core.WorkList{Count: h.IssueCount, Items: convert(h.Nodes, workNode.core)}
}

// ViewerWork returns the open work waiting on the signed-in user: the pull
// requests that ask for their review, those they opened, and the issues
// assigned to them, each with its count and its first most recently
// updated items. First is at most 100.
func (c *Client) ViewerWork(ctx context.Context, first int) (core.Work, error) {
	var data struct {
		ReviewRequested workHits `json:"reviewRequested"`
		Authored        workHits `json:"authored"`
		Assigned        workHits `json:"assigned"`
	}
	if err := c.Query(ctx, viewerWorkQuery, map[string]any{"first": first}, &data); err != nil {
		return core.Work{}, fmt.Errorf("viewer work: %w", err)
	}
	return core.Work{
		ReviewRequested: data.ReviewRequested.core(),
		Authored:        data.Authored.core(),
		Assigned:        data.Assigned.core(),
	}, nil
}
