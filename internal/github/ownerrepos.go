package github

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
)

// The owner lists sort like ListRepos, most recently updated first, so that
// the time a row shows follows the order.

const viewerOwnReposQuery = `query ViewerOwnRepos($first: Int!, $after: String) {
  ` + rateLimitField + `
  viewer {
    repositories(
      first: $first
      after: $after
      orderBy: {field: UPDATED_AT, direction: DESC}
      ownerAffiliations: [OWNER]
    ) {
      nodes { ...repoFields }
      pageInfo { hasNextPage endCursor }
    }
  }
}
` + repoFields

const orgReposQuery = `query OrgRepos($login: String!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  organization(login: $login) {
    repositories(first: $first, after: $after, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes { ...repoFields }
      pageInfo { hasNextPage endCursor }
    }
  }
}
` + repoFields

// ownerRepos is a page of an owner's repositories.
type ownerRepos struct {
	nodes[repoNode]
	PageInfo pageInfo `json:"pageInfo"`
}

func (r *ownerRepos) core() core.Page[core.Repo] {
	return core.Page[core.Repo]{Items: convert(r.Nodes, repoNode.core), Next: r.PageInfo.next()}
}

// ownerReposVars are the variables of a page of first repositories after a
// cursor. The first page has a null cursor.
func ownerReposVars(first int, after string) map[string]any {
	vars := map[string]any{"first": first, "after": nil}
	if after != "" {
		vars["after"] = after
	}
	return vars
}

// ViewerOwnRepos returns a page of up to first repositories that the
// signed-in user owns, most recently updated first. After is the Next of
// the previous page, or empty for the first page.
func (c *Client) ViewerOwnRepos(ctx context.Context, first int, after string) (core.Page[core.Repo], error) {
	var data struct {
		Viewer struct {
			Repositories ownerRepos `json:"repositories"`
		} `json:"viewer"`
	}
	if err := c.Query(ctx, viewerOwnReposQuery, ownerReposVars(first, after), &data); err != nil {
		return core.Page[core.Repo]{}, fmt.Errorf("list own repos: %w", err)
	}
	return data.Viewer.Repositories.core(), nil
}

// OrgRepos returns a page of up to first repositories of the organization
// login that the viewer can see, most recently updated first. After works
// as in ViewerOwnRepos. It returns an error matching core.ErrNotFound if
// there is no such organization.
func (c *Client) OrgRepos(ctx context.Context, login string, first int, after string) (core.Page[core.Repo], error) {
	var data struct {
		Organization *struct {
			Repositories ownerRepos `json:"repositories"`
		} `json:"organization"`
	}
	vars := ownerReposVars(first, after)
	vars["login"] = login
	if err := c.Query(ctx, orgReposQuery, vars, &data); err != nil {
		return core.Page[core.Repo]{}, fmt.Errorf("list repos of %s: %w", login, err)
	}
	if data.Organization == nil {
		return core.Page[core.Repo]{}, fmt.Errorf("list repos of %s: %w", login, core.ErrNotFound)
	}
	return data.Organization.Repositories.core(), nil
}
