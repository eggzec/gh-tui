package github

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// repoUsersQuery reads the people who can be assigned to a repository's
// issues, who are the likeliest authors and reviewers, and, once the user
// types, the accounts whose login starts with it too, since anyone may
// open an issue. Both cost one point.
var repoUsersQuery = `query RepoUsers($owner: String!, $name: String!, $query: String, $first: Int!, $search: String!, $typed: Boolean!) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    assignableUsers(query: $query, first: $first) { nodes { login name } }
  }
  search(type: USER, query: $search, first: $first) @include(if: $typed) {
    nodes { ... on User { login name } }
  }
}
`

// RepoUsers returns up to first people of repo who match query, a part of
// a login or a name: those who can be assigned first, then other accounts.
// An empty query returns the first who can be assigned.
func (c *Client) RepoUsers(ctx context.Context, repo core.RepoRef, query string, first int) ([]core.User, error) {
	query = strings.TrimSpace(query)
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "first": first, "search": "", "typed": query != ""}
	if query != "" {
		vars["query"] = query
		vars["search"] = query + " in:login"
	}
	var data struct {
		Repository *struct {
			AssignableUsers nodes[user] `json:"assignableUsers"`
		} `json:"repository"`
		Search *nodes[user] `json:"search"`
	}
	if err := c.Query(ctx, repoUsersQuery, vars, &data); err != nil {
		return nil, fmt.Errorf("list people of %s: %w", repo, err)
	}
	if data.Repository == nil {
		return nil, fmt.Errorf("list people of %s: %w", repo, core.ErrNotFound)
	}
	found := data.Repository.AssignableUsers.Nodes
	if data.Search != nil {
		found = append(found, data.Search.Nodes...)
	}
	out := make([]core.User, 0, len(found))
	for _, u := range found {
		// An organization matches no fragment, so it decodes empty.
		if u.Login == "" || slices.ContainsFunc(out, func(o core.User) bool { return strings.EqualFold(o.Login, u.Login) }) {
			continue
		}
		out = append(out, u.core())
	}
	return out, nil
}

const viewerLoginQuery = `query ViewerLogin {
  ` + rateLimitField + `
  viewer { login }
}
`

// ViewerLogin returns the login of the signed-in user.
func (c *Client) ViewerLogin(ctx context.Context) (string, error) {
	var data struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
	}
	if err := c.Query(ctx, viewerLoginQuery, nil, &data); err != nil {
		return "", fmt.Errorf("read viewer: %w", err)
	}
	return data.Viewer.Login, nil
}
