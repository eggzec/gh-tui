package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/eggzec/gh-tui/internal/core"
)

// userFields and orgFields are the fields of a row of a list of users or
// of organizations. The bio is a user's; an organization has a
// description instead.
const (
	userFields = `__typename login name bio`
	orgFields  = `__typename login name description`
)

// userFollowersQuery lists the users who follow a user.
const userFollowersQuery = `query UserFollowers($login: String!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  user(login: $login) {
    followers(first: $first, after: $after) {
      nodes { ` + userFields + ` }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

// userFollowingQuery lists the users a user follows.
const userFollowingQuery = `query UserFollowing($login: String!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  user(login: $login) {
    following(first: $first, after: $after) {
      nodes { ` + userFields + ` }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

// userOrgsQuery lists the organizations a user belongs to: the public
// memberships, and those the viewer shares.
const userOrgsQuery = `query UserOrgs($login: String!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  user(login: $login) {
    organizations(first: $first, after: $after) {
      nodes { ` + orgFields + ` }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

// orgMembersQuery lists an organization's members. Someone outside the
// organization sees its public members only, without their roles.
const orgMembersQuery = `query OrgMembers($login: String!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  organization(login: $login) {
    membersWithRole(first: $first, after: $after) {
      edges {
        role
        node { ` + userFields + ` }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

// orgTeamsQuery lists an organization's teams by name. Only members see
// them, so it asks whether the viewer is one.
const orgTeamsQuery = `query OrgTeams($login: String!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  organization(login: $login) {
    viewerIsAMember
    teams(first: $first, after: $after, orderBy: {field: NAME, direction: ASC}) {
      nodes {
        name
        slug
        description
        privacy
        members { totalCount }
        url
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

// userStarsQuery lists the repositories a user starred, the latest first.
const userStarsQuery = `query UserStars($login: String!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  user(login: $login) {
    starredRepositories(first: $first, after: $after, orderBy: {field: STARRED_AT, direction: DESC}) {
      nodes { ...repoFields }
      pageInfo { hasNextPage endCursor }
    }
  }
}
` + repoFields

// personNode is a row of userFields or orgFields.
type personNode struct {
	Typename    string `json:"__typename"`
	Login       string `json:"login"`
	Name        string `json:"name"`
	Bio         string `json:"bio"`
	Description string `json:"description"`
}

func (p personNode) core() core.Person {
	out := core.Person{Login: p.Login, Name: p.Name, Bio: p.Bio}
	if p.Typename == "Organization" {
		out.Kind = core.OwnerOrg
		out.Bio = p.Description
	}
	return out
}

// people is a page of accounts.
type people struct {
	nodes[personNode]
	PageInfo pageInfo `json:"pageInfo"`
}

func (p *people) core() core.Page[core.Person] {
	return core.Page[core.Person]{Items: convert(p.Nodes, personNode.core), Next: p.PageInfo.next()}
}

// memberRoles are the core roles of GitHub's OrganizationMemberRole.
var memberRoles = map[string]core.MemberRole{
	"ADMIN":  core.MemberRoleAdmin,
	"MEMBER": core.MemberRoleMember,
}

// UserFollowers returns a page of up to first users who follow the user
// login. After is the Next of the previous page, or empty for the first
// page. It returns an error matching core.ErrNotFound if there is no such
// user.
func (c *Client) UserFollowers(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	return c.userPeople(ctx, userFollowersQuery, "followers of", login, first, after, func(u *userLists) *people { return u.Followers })
}

// UserFollowing returns a page of the users the user login follows, as
// UserFollowers does.
func (c *Client) UserFollowing(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	return c.userPeople(ctx, userFollowingQuery, "following of", login, first, after, func(u *userLists) *people { return u.Following })
}

// UserOrgs returns a page of the organizations the user login belongs to,
// as UserFollowers does: those the user shows publicly, and those the
// viewer belongs to as well.
func (c *Client) UserOrgs(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	return c.userPeople(ctx, userOrgsQuery, "organizations of", login, first, after, func(u *userLists) *people { return u.Organizations })
}

// userLists are the lists of accounts that the queries of userPeople name
// by an alias, one each.
type userLists struct {
	Followers     *people `json:"followers"`
	Following     *people `json:"following"`
	Organizations *people `json:"organizations"`
}

// userPeople reads a page of query, one of the user's lists of accounts,
// which pick takes from the answer. A query that answers with another
// list is an error, not an empty page.
func (c *Client) userPeople(ctx context.Context, query, what, login string, first int, after string, pick func(*userLists) *people) (core.Page[core.Person], error) {
	var data struct {
		User *userLists `json:"user"`
	}
	vars := ownerReposVars(first, after)
	vars["login"] = login
	if err := c.Query(ctx, query, vars, &data); err != nil {
		return core.Page[core.Person]{}, fmt.Errorf("list %s %s: %w", what, login, err)
	}
	if data.User == nil {
		return core.Page[core.Person]{}, fmt.Errorf("list %s %s: %w", what, login, core.ErrNotFound)
	}
	p := pick(data.User)
	if p == nil {
		return core.Page[core.Person]{}, fmt.Errorf("list %s %s: the answer has no such list", what, login)
	}
	return p.core(), nil
}

// OrgMembers returns a page of up to first members of the organization
// login, with their roles. Someone outside the organization gets its
// public members, with their roles unknown. After works as in
// UserFollowers. It returns an error matching core.ErrNotFound if there
// is no such organization.
func (c *Client) OrgMembers(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	var data struct {
		Org *struct {
			Members struct {
				Edges []struct {
					Role string     `json:"role"`
					Node personNode `json:"node"`
				} `json:"edges"`
				PageInfo pageInfo `json:"pageInfo"`
			} `json:"membersWithRole"`
		} `json:"organization"`
	}
	vars := ownerReposVars(first, after)
	vars["login"] = login
	if err := c.Query(ctx, orgMembersQuery, vars, &data); err != nil {
		return core.Page[core.Person]{}, fmt.Errorf("list members of %s: %w", login, err)
	}
	if data.Org == nil {
		return core.Page[core.Person]{}, fmt.Errorf("list members of %s: %w", login, core.ErrNotFound)
	}
	m := data.Org.Members
	page := core.Page[core.Person]{Next: m.PageInfo.next()}
	for _, e := range m.Edges {
		p := e.Node.core()
		p.Role = memberRoles[e.Role]
		page.Items = append(page.Items, p)
	}
	return page, nil
}

// errMembersOnly is why someone outside an organization can't list its
// teams.
var errMembersOnly = fmt.Errorf("only members see the teams: %w", core.ErrForbidden)

// OrgTeams returns a page of up to first teams of the organization login,
// by name. Only members see the teams: for anyone else it returns an
// error matching core.ErrForbidden. After works as in UserFollowers. It
// returns an error matching core.ErrNotFound if there is no such
// organization.
func (c *Client) OrgTeams(ctx context.Context, login string, first int, after string) (core.Page[core.Team], error) {
	var data struct {
		Org *struct {
			Member bool `json:"viewerIsAMember"`
			Teams  struct {
				Nodes []struct {
					Name        string      `json:"name"`
					Slug        string      `json:"slug"`
					Description string      `json:"description"`
					Privacy     string      `json:"privacy"`
					Members     viewerCount `json:"members"`
					URL         string      `json:"url"`
				} `json:"nodes"`
				PageInfo pageInfo `json:"pageInfo"`
			} `json:"teams"`
		} `json:"organization"`
	}
	vars := ownerReposVars(first, after)
	vars["login"] = login
	if err := c.Query(ctx, orgTeamsQuery, vars, &data); err != nil {
		return core.Page[core.Team]{}, fmt.Errorf("list teams of %s: %w", login, err)
	}
	if data.Org == nil {
		return core.Page[core.Team]{}, fmt.Errorf("list teams of %s: %w", login, core.ErrNotFound)
	}
	if !data.Org.Member {
		return core.Page[core.Team]{}, fmt.Errorf("list teams of %s: %w", login, errMembersOnly)
	}
	t := data.Org.Teams
	page := core.Page[core.Team]{Next: t.PageInfo.next()}
	for _, n := range t.Nodes {
		page.Items = append(page.Items, core.Team{
			Name:        n.Name,
			Slug:        n.Slug,
			Description: n.Description,
			Secret:      n.Privacy == "SECRET",
			Members:     n.Members.TotalCount,
			URL:         n.URL,
		})
	}
	return page, nil
}

// UserStars returns a page of up to first repositories the user login
// starred, the latest first. After works as in UserFollowers. It returns
// an error matching core.ErrNotFound if there is no such user.
func (c *Client) UserStars(ctx context.Context, login string, first int, after string) (core.Page[core.Repo], error) {
	var data struct {
		User *struct {
			Starred ownerRepos `json:"starredRepositories"`
		} `json:"user"`
	}
	vars := ownerReposVars(first, after)
	vars["login"] = login
	if err := c.Query(ctx, userStarsQuery, vars, &data); err != nil {
		return core.Page[core.Repo]{}, fmt.Errorf("list stars of %s: %w", login, err)
	}
	if data.User == nil {
		return core.Page[core.Repo]{}, fmt.Errorf("list stars of %s: %w", login, core.ErrNotFound)
	}
	return data.User.Starred.core(), nil
}

// sponsorFields are the fields of a sponsor or a sponsored account, a
// user or an organization.
const sponsorFields = `__typename
        ... on User { login name bio }
        ... on Organization { login name description }`

// ownerSponsorsQuery lists the public sponsors of a user or an
// organization. GitHub Sponsors is github.com's only, so it is never sent
// to an Enterprise Server.
const ownerSponsorsQuery = `query OwnerSponsors($login: String!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  repositoryOwner(login: $login) {
    ... on Sponsorable {
      sponsors(first: $first, after: $after) {
        nodes { ` + sponsorFields + ` }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

// ownerSponsoringQuery lists whom a user or an organization sponsors in
// public, on github.com only, as ownerSponsorsQuery.
const ownerSponsoringQuery = `query OwnerSponsoring($login: String!, $first: Int!, $after: String) {
  ` + rateLimitField + `
  repositoryOwner(login: $login) {
    ... on Sponsorable {
      sponsoring(first: $first, after: $after) {
        nodes { ` + sponsorFields + ` }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

// OwnerSponsors returns a page of up to first public sponsors of the user
// or organization login. After works as in UserFollowers. GitHub
// Sponsors is github.com's only: on an Enterprise Server it returns an
// error matching core.ErrUnsupported without asking. It returns an error
// matching core.ErrNotFound if there is no such account.
func (c *Client) OwnerSponsors(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	return c.sponsorPeople(ctx, ownerSponsorsQuery, "sponsors of", login, first, after)
}

// OwnerSponsoring returns a page of the accounts the user or organization
// login sponsors in public, as OwnerSponsors does.
func (c *Client) OwnerSponsoring(ctx context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	return c.sponsorPeople(ctx, ownerSponsoringQuery, "sponsoring of", login, first, after)
}

func (c *Client) sponsorPeople(ctx context.Context, query, what, login string, first int, after string) (core.Page[core.Person], error) {
	if c.enterprise.Load() {
		return core.Page[core.Person]{}, fmt.Errorf("list %s %s: GitHub Sponsors is on github.com only: %w", what, login, core.ErrUnsupported)
	}
	var data struct {
		Owner *struct {
			Sponsors   *people `json:"sponsors"`
			Sponsoring *people `json:"sponsoring"`
		} `json:"repositoryOwner"`
	}
	vars := ownerReposVars(first, after)
	vars["login"] = login
	if err := c.Query(ctx, query, vars, &data); err != nil {
		return core.Page[core.Person]{}, fmt.Errorf("list %s %s: %w", what, login, err)
	}
	if data.Owner == nil {
		return core.Page[core.Person]{}, fmt.Errorf("list %s %s: %w", what, login, core.ErrNotFound)
	}
	for _, p := range []*people{data.Owner.Sponsors, data.Owner.Sponsoring} {
		if p != nil {
			return p.core(), nil
		}
	}
	return core.Page[core.Person]{}, nil
}

// maxReadme is the most of a profile README that is read.
const maxReadme = 1 << 20

// ProfileReadme returns the profile README of the owner login of kind:
// a user's is in the repository named after them, and an organization's
// in the profile directory of its .github repository. For a member of an
// organization, member set, the README of its .github-private repository,
// which only members see, comes first, and one the token may not read is
// skipped. An owner without a README is not an error: the Readme's Source
// is then empty. If cond is current, the Response has NotModified set and
// the Readme is empty; pass the validators of the README last returned.
// The caller must keep that README's Source, which a 304 doesn't repeat,
// and must read again without validators when member changes: the same
// README in both repositories has the same ETag, so a 304 could be for
// either of them.
func (c *Client) ProfileReadme(ctx context.Context, login string, kind core.OwnerKind, member bool, cond Conditional) (core.Readme, Response, error) {
	var tries []core.RepoRef
	switch {
	case kind == core.OwnerUser:
		tries = []core.RepoRef{{Owner: login, Name: login}}
	case member:
		tries = []core.RepoRef{{Owner: login, Name: ".github-private"}, {Owner: login, Name: ".github"}}
	default:
		tries = []core.RepoRef{{Owner: login, Name: ".github"}}
	}
	var res Response
	for _, repo := range tries {
		b, r, err := c.readme(ctx, repo, kind == core.OwnerOrg, cond)
		res = r
		switch {
		case errors.Is(err, core.ErrNotFound):
			continue
		case errors.Is(err, core.ErrForbidden) && repo.Name == ".github-private":
			// A member whose token may not read the private profile
			// still sees the public one.
			continue
		case err != nil:
			return core.Readme{}, res, fmt.Errorf("profile README of %s: %w", login, err)
		case res.NotModified:
			return core.Readme{}, res, nil
		}
		return core.Readme{Markdown: string(b), Source: repo, MembersOnly: repo.Name == ".github-private"}, res, nil
	}
	return core.Readme{}, res, nil
}

// readme reads the README of repo, of its profile directory if profile is
// set, as raw content.
func (c *Client) readme(ctx context.Context, repo core.RepoRef, profile bool, cond Conditional) ([]byte, Response, error) {
	path := "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) + "/readme"
	if profile {
		path += "/profile"
	}
	return c.getRaw(ctx, path, cond, maxReadme)
}

// restFollowers is the follower count of an account's REST answer.
type restFollowers struct {
	Followers int `json:"followers"`
}

// OrgFollowers returns how many follow the organization login, which
// GitHub's GraphQL API doesn't count. It returns an error matching
// core.ErrNotFound if there is no such account.
func (c *Client) OrgFollowers(ctx context.Context, login string) (int, error) {
	var r restFollowers
	if _, err := c.Get(ctx, "users/"+url.PathEscape(login), Conditional{}, &r); err != nil {
		return 0, fmt.Errorf("followers of %s: %w", login, err)
	}
	return r.Followers, nil
}
