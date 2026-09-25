package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
  primaryLanguage { name color }
  stargazerCount
  viewerHasStarred
  isPrivate
  isFork
  isArchived
  isTemplate
  isMirror
  updatedAt
  url
}`

// ownerAffiliations must list ORGANIZATION_MEMBER too: it defaults to OWNER
// and COLLABORATOR, which would hide the repositories of the viewer's
// organizations even though affiliations asks for them.
const listReposQuery = `query ListRepos($first: Int!, $after: String) {
  ` + rateLimitField + `
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

// repoCapsFields selects what core.RepoCaps holds. Only the read of one
// repository asks for them: they cost nothing, but a list has no use for
// them.
const repoCapsFields = `fragment repoCapsFields on Repository {
  viewerPermission
  isLocked
  hasIssuesEnabled
  hasPullRequestsEnabled
  hasDiscussionsEnabled
  hasProjectsEnabled
  hasWikiEnabled
  mergeCommitAllowed
  squashMergeAllowed
  rebaseMergeAllowed
  autoMergeAllowed
  viewerDefaultMergeMethod
}`

const getRepoQuery = `query GetRepo($owner: String!, $name: String!) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) { ...repoFields ...repoCapsFields }
}
` + repoFields + "\n" + repoCapsFields

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
	PrimaryLanguage  *repoLanguage `json:"primaryLanguage"`
	StargazerCount   int           `json:"stargazerCount"`
	ViewerHasStarred bool          `json:"viewerHasStarred"`
	IsPrivate        bool          `json:"isPrivate"`
	IsFork           bool          `json:"isFork"`
	IsArchived       bool          `json:"isArchived"`
	IsTemplate       bool          `json:"isTemplate"`
	IsMirror         bool          `json:"isMirror"`
	UpdatedAt        time.Time     `json:"updatedAt"`
	URL              string        `json:"url"`
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
		Template:    n.IsTemplate,
		Mirror:      n.IsMirror,
		UpdatedAt:   n.UpdatedAt,
		URL:         n.URL,
	}
	if n.DefaultBranchRef != nil {
		r.DefaultBranch = n.DefaultBranchRef.Name
	}
	if n.PrimaryLanguage != nil {
		r.Language, r.LanguageColor = n.PrimaryLanguage.Name, n.PrimaryLanguage.Color
	}
	return r
}

// repoDetail is the JSON shape of the repository in getRepoQuery.
type repoDetail struct {
	repoNode
	// ViewerPermission is null for a GitHub App.
	ViewerPermission         string `json:"viewerPermission"`
	IsLocked                 bool   `json:"isLocked"`
	HasIssuesEnabled         bool   `json:"hasIssuesEnabled"`
	HasPullRequestsEnabled   bool   `json:"hasPullRequestsEnabled"`
	HasDiscussionsEnabled    bool   `json:"hasDiscussionsEnabled"`
	HasProjectsEnabled       bool   `json:"hasProjectsEnabled"`
	HasWikiEnabled           bool   `json:"hasWikiEnabled"`
	MergeCommitAllowed       bool   `json:"mergeCommitAllowed"`
	SquashMergeAllowed       bool   `json:"squashMergeAllowed"`
	RebaseMergeAllowed       bool   `json:"rebaseMergeAllowed"`
	AutoMergeAllowed         bool   `json:"autoMergeAllowed"`
	ViewerDefaultMergeMethod string `json:"viewerDefaultMergeMethod"`
}

func (d repoDetail) core() core.Repo {
	r := d.repoNode.core()
	r.Caps = core.RepoCaps{
		Known:        true,
		Permission:   permission(d.ViewerPermission),
		Archived:     d.IsArchived,
		Locked:       d.IsLocked,
		Issues:       d.HasIssuesEnabled,
		PullRequests: d.HasPullRequestsEnabled,
		Discussions:  d.HasDiscussionsEnabled,
		Projects:     d.HasProjectsEnabled,
		Wiki:         d.HasWikiEnabled,
		MergeCommit:  d.MergeCommitAllowed,
		Squash:       d.SquashMergeAllowed,
		Rebase:       d.RebaseMergeAllowed,
		AutoMerge:    d.AutoMergeAllowed,
		DefaultMerge: core.MergeMethod(strings.ToLower(d.ViewerDefaultMergeMethod)),
	}
	return r
}

// permission maps a GraphQL RepositoryPermission. TRIAGE_PLUS is triage
// with a little more, which counts as triage here.
func permission(s string) core.Permission {
	if s == "TRIAGE_PLUS" {
		return core.PermissionTriage
	}
	return core.Permission(strings.ToLower(s))
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

// GetRepo returns one repository with what the viewer may do in it. It
// returns an error matching core.ErrNotFound if the repository doesn't
// exist or the viewer can't see it.
func (c *Client) GetRepo(ctx context.Context, ref core.RepoRef) (core.Repo, error) {
	var data struct {
		Repository *repoDetail `json:"repository"`
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
