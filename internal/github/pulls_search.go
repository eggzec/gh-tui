package github

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
)

// searchPullsQuery searches pull requests for the fields of a list row, so
// that what a repository's list can't select, such as the author or the
// review, is found at the cost of one point too.
var searchPullsQuery = `query SearchPulls($query: String!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  search(type: ISSUE, query: $query, first: $first, after: $after) {
    nodes { ... on PullRequest { ...pullFields } }
    pageInfo { hasNextPage endCursor }
  }
}
` + pullFields

// SearchPullRequests returns a page of up to first pull requests that query
// matches, in GitHub's search syntax, such as "repo:o/r is:pr is:open
// author:@me sort:updated-desc". Without a sort: qualifier GitHub orders
// them by best match. Issues the query matches are left out, so add is:pr.
// Cursor and first work as in ListPullRequests.
func (c *Client) SearchPullRequests(ctx context.Context, query, cursor string, first int) (core.Page[core.PullRequest], error) {
	vars := map[string]any{"query": query, "first": first}
	if cursor != "" {
		vars["after"] = cursor
	}
	var data struct {
		Search struct {
			Nodes    []pull   `json:"nodes"`
			PageInfo pageInfo `json:"pageInfo"`
		} `json:"search"`
	}
	if err := c.Query(ctx, searchPullsQuery, vars, &data); err != nil {
		return core.Page[core.PullRequest]{}, fmt.Errorf("search pull requests: %w", err)
	}
	page := core.Page[core.PullRequest]{Next: data.Search.PageInfo.next()}
	for i := range data.Search.Nodes {
		// An issue matches no fragment, so it decodes empty.
		if n := &data.Search.Nodes[i]; n.ID != "" {
			page.Items = append(page.Items, n.core())
		}
	}
	return page, nil
}
