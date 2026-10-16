package github

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
)

// codeownersPaths are the places GitHub looks for a CODEOWNERS file, in the
// order it does: the first that exists is the one that counts.
var codeownersPaths = [...]string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}

// codeownersQuery reads the three places of a CODEOWNERS file at one
// commit, each as an object named by $expression. A blob that isn't text,
// or a path that is no blob, answers null.
const codeownersQuery = `query Codeowners($owner: String!, $name: String!, $github: String!, $root: String!, $docs: String!) {
  ` + rateLimitField + `
  repository(owner: $owner, name: $name) {
    github: object(expression: $github) { ... on Blob { text } }
    root: object(expression: $root) { ... on Blob { text } }
    docs: object(expression: $docs) { ... on Blob { text } }
  }
}`

// codeownersBlob is what one of the aliases of codeownersQuery answers.
type codeownersBlob struct {
	Text *string `json:"text"`
}

// GetCodeowners returns the CODEOWNERS file of repo as it was at the commit
// base. A commit's file never changes. It is the one in .github, else in the
// root, else in docs, as GitHub chooses. A commit without one gives an empty
// file, not an error. It returns an error matching core.ErrNotFound if
// there is no such repository.
func (c *Client) GetCodeowners(ctx context.Context, repo core.RepoRef, base string) (core.CodeownersFile, error) {
	if base == "" {
		return core.CodeownersFile{}, fmt.Errorf("read CODEOWNERS of %s: no commit", repo)
	}
	vars := map[string]any{
		"owner":  repo.Owner,
		"name":   repo.Name,
		"github": base + ":" + codeownersPaths[0],
		"root":   base + ":" + codeownersPaths[1],
		"docs":   base + ":" + codeownersPaths[2],
	}
	var data struct {
		Repository *struct {
			Github *codeownersBlob `json:"github"`
			Root   *codeownersBlob `json:"root"`
			Docs   *codeownersBlob `json:"docs"`
		} `json:"repository"`
	}
	if err := c.Query(ctx, codeownersQuery, vars, &data); err != nil {
		return core.CodeownersFile{}, fmt.Errorf("read CODEOWNERS of %s: %w", repo, err)
	}
	if data.Repository == nil {
		return core.CodeownersFile{}, fmt.Errorf("read CODEOWNERS of %s: %w", repo, core.ErrNotFound)
	}
	found := [...]*codeownersBlob{data.Repository.Github, data.Repository.Root, data.Repository.Docs}
	for i, b := range found {
		if b != nil && b.Text != nil {
			return core.CodeownersFile{Path: codeownersPaths[i], Text: *b.Text}, nil
		}
	}
	return core.CodeownersFile{}, nil
}

// teamMembersPerPage is how many members a page of a team lists: the most
// GitHub gives.
const teamMembersPerPage = 100

// MaxTeamMembers is how many members of a team ListTeamMembers reads at
// most. A team that has more is listed Truncated.
const MaxTeamMembers = 500

// restTeamMember is a member in the answer of a team's members.
type restTeamMember struct {
	Login string `json:"login"`
}

func (m restTeamMember) core() core.Person {
	return core.Person{Login: m.Login}
}

// ListTeamMembers returns the members of the team slug of the organization
// org, as GitHub lists them, reading as many pages as it takes, up to
// MaxTeamMembers. The page has no Next, and is Truncated if the team has
// more. GitHub answers 404, which is an error matching core.ErrNotFound,
// for a team the token may not see, such as any without the read:org scope,
// and 403, which matches core.ErrForbidden, when the token has no permission
// to read the organization's members or its SSO doesn't authorize it.
func (c *Client) ListTeamMembers(ctx context.Context, org, slug string) (core.Page[core.Person], error) {
	path := "/orgs/" + url.PathEscape(org) + "/teams/" + url.PathEscape(slug) + "/members?per_page=" + strconv.Itoa(teamMembersPerPage)
	var (
		items     []restTeamMember
		truncated bool
	)
	for path != "" {
		var got []restTeamMember
		res, err := c.Get(ctx, path, Conditional{}, &got)
		if err != nil {
			return core.Page[core.Person]{}, fmt.Errorf("list members of team %s/%s: %w", org, slug, err)
		}
		items = append(items, got...)
		if len(items) >= MaxTeamMembers {
			// A last page that ends at the limit has nothing after it.
			truncated = len(items) > MaxTeamMembers || res.Next != ""
			items = items[:MaxTeamMembers]
			break
		}
		path = res.Next
	}
	return core.Page[core.Person]{Items: convert(items, restTeamMember.core), Truncated: truncated}, nil
}
