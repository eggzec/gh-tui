package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// repoFields selects what core.Repo holds. The list and the detail share it
// so that both decode into repoNode.
const repoFields = `fragment repoFields on Repository {
  id
  name
  owner { login }
  description
  defaultBranchRef { name }
  primaryLanguage { name }
  stargazerCount
  viewerHasStarred
  isPrivate
  isFork
  isArchived
  updatedAt
  url
}`

// ownerAffiliations must list ORGANIZATION_MEMBER too: it defaults to OWNER
// and COLLABORATOR, which would hide the repositories of the viewer's
// organizations even though affiliations asks for them.
const listReposQuery = `query($first: Int!, $after: String) {
  viewer {
    repositories(
      first: $first
      after: $after
      orderBy: {field: UPDATED_AT, direction: DESC}
      affiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER]
      ownerAffiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER]
    ) {
      nodes { ...repoFields }
      pageInfo { hasNextPage endCursor }
    }
  }
}
` + repoFields

const getRepoQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) { ...repoFields }
}
` + repoFields

type repoNode struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	Description string `json:"description"`
	// An empty repository has no default branch and may have no language.
	DefaultBranchRef *struct {
		Name string `json:"name"`
	} `json:"defaultBranchRef"`
	PrimaryLanguage *struct {
		Name string `json:"name"`
	} `json:"primaryLanguage"`
	StargazerCount   int       `json:"stargazerCount"`
	ViewerHasStarred bool      `json:"viewerHasStarred"`
	IsPrivate        bool      `json:"isPrivate"`
	IsFork           bool      `json:"isFork"`
	IsArchived       bool      `json:"isArchived"`
	UpdatedAt        time.Time `json:"updatedAt"`
	URL              string    `json:"url"`
}

func (n repoNode) core() core.Repo {
	r := core.Repo{
		ID:          n.ID,
		Ref:         core.RepoRef{Owner: n.Owner.Login, Name: n.Name},
		Description: n.Description,
		Stars:       n.StargazerCount,
		Starred:     n.ViewerHasStarred,
		Private:     n.IsPrivate,
		Fork:        n.IsFork,
		Archived:    n.IsArchived,
		UpdatedAt:   n.UpdatedAt,
		URL:         n.URL,
	}
	if n.DefaultBranchRef != nil {
		r.DefaultBranch = n.DefaultBranchRef.Name
	}
	if n.PrimaryLanguage != nil {
		r.Language = n.PrimaryLanguage.Name
	}
	return r
}

// ListRepos returns a page of up to first repositories that the viewer owns,
// collaborates on or can access as an organization member, most recently
// updated first. After is the Next cursor of the previous page, or empty for
// the first page.
func (c *Client) ListRepos(ctx context.Context, first int, after string) (core.Page[core.Repo], error) {
	vars := map[string]any{"first": first, "after": nil}
	if after != "" {
		vars["after"] = after
	}
	var data struct {
		Viewer struct {
			Repositories struct {
				nodes[repoNode]
				PageInfo pageInfo `json:"pageInfo"`
			} `json:"repositories"`
		} `json:"viewer"`
	}
	if err := c.Query(ctx, listReposQuery, vars, &data); err != nil {
		return core.Page[core.Repo]{}, fmt.Errorf("list repos: %w", err)
	}
	conn := data.Viewer.Repositories
	return core.Page[core.Repo]{
		Items: convert(conn.Nodes, repoNode.core),
		Next:  conn.PageInfo.next(),
	}, nil
}

// GetRepo returns one repository. It returns an error matching
// core.ErrNotFound if the repository doesn't exist or the viewer can't see
// it.
func (c *Client) GetRepo(ctx context.Context, ref core.RepoRef) (core.Repo, error) {
	var data struct {
		Repository *repoNode `json:"repository"`
	}
	vars := map[string]any{"owner": ref.Owner, "name": ref.Name}
	if err := c.Query(ctx, getRepoQuery, vars, &data); err != nil {
		return core.Repo{}, fmt.Errorf("get repo %s: %w", ref, err)
	}
	if data.Repository == nil {
		return core.Repo{}, fmt.Errorf("get repo %s: %w", ref, core.ErrNotFound)
	}
	return data.Repository.core(), nil
}

// Star stars a repository for the viewer. Starring it again does nothing.
func (c *Client) Star(ctx context.Context, ref core.RepoRef) error {
	_, err := c.Do(ctx, http.MethodPut, starredPath(ref), nil, nil)
	return err
}

// Unstar removes the viewer's star from a repository. Unstarring a
// repository that isn't starred does nothing.
func (c *Client) Unstar(ctx context.Context, ref core.RepoRef) error {
	_, err := c.Do(ctx, http.MethodDelete, starredPath(ref), nil, nil)
	return err
}

func starredPath(ref core.RepoRef) string {
	return "user/starred/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Name)
}
