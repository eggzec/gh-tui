package github

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// ownerHeaderQuery reads the top of a user's or an organization's page in
// one request. Pinned items may be gists too, so types keeps them to
// repositories. Sponsorship fields exist on github.com only, so they are
// not asked for here.
const ownerHeaderQuery = `query OwnerHeader($login: String!, $pinned: Int!) {
  ` + rateLimitField + `
  repositoryOwner(login: $login) {
    __typename
    id
    login
    url
    avatarUrl
    ... on User {
      name
      bio
      company
      location
      websiteUrl
      pronouns
      twitterUsername
      createdAt
      status { emoji message indicatesLimitedAvailability }
      followers { totalCount }
      following { totalCount }
      starredRepositories { totalCount }
      repositories(ownerAffiliations: [OWNER]) { totalCount }
      pinnedItems(first: $pinned, types: [REPOSITORY]) {
        nodes { ...repoFields }
      }
      isViewer
      viewerIsFollowing
      viewerCanFollow
      isFollowingViewer
    }
    ... on Organization {
      name
      description
      location
      websiteUrl
      email
      twitterUsername
      isVerified
      createdAt
      repositories { totalCount }
      membersWithRole { totalCount }
      teams { totalCount }
      pinnedItems(first: $pinned, types: [REPOSITORY]) {
        nodes { ...repoFields }
      }
      viewerIsAMember
      viewerIsFollowing
      viewerCanAdminister
    }
  }
}
` + repoFields

// ownerNode is the repositoryOwner of ownerHeaderQuery, with the fields of
// both kinds; those of the other kind are absent.
type ownerNode struct {
	Typename    string    `json:"__typename"`
	ID          string    `json:"id"`
	Login       string    `json:"login"`
	URL         string    `json:"url"`
	AvatarURL   string    `json:"avatarUrl"`
	Name        string    `json:"name"`
	Bio         string    `json:"bio"`
	Description string    `json:"description"`
	Company     string    `json:"company"`
	Location    string    `json:"location"`
	WebsiteURL  string    `json:"websiteUrl"`
	Pronouns    string    `json:"pronouns"`
	Twitter     string    `json:"twitterUsername"`
	Email       string    `json:"email"`
	Verified    bool      `json:"isVerified"`
	CreatedAt   time.Time `json:"createdAt"`
	Status      *struct {
		Emoji   string `json:"emoji"`
		Message string `json:"message"`
		Busy    bool   `json:"indicatesLimitedAvailability"`
	} `json:"status"`
	Followers     viewerCount      `json:"followers"`
	Following     viewerCount      `json:"following"`
	Starred       viewerCount      `json:"starredRepositories"`
	Repositories  viewerCount      `json:"repositories"`
	Members       viewerCount      `json:"membersWithRole"`
	Teams         viewerCount      `json:"teams"`
	PinnedItems   nodes[*repoNode] `json:"pinnedItems"`
	IsViewer      bool             `json:"isViewer"`
	ViewerFollows bool             `json:"viewerIsFollowing"`
	CanFollow     bool             `json:"viewerCanFollow"`
	FollowsViewer bool             `json:"isFollowingViewer"`
	Member        bool             `json:"viewerIsAMember"`
	CanAdminister bool             `json:"viewerCanAdminister"`
}

func (o *ownerNode) core() core.Owner {
	p := core.Profile{
		Login:     o.Login,
		Name:      o.Name,
		Bio:       o.Bio,
		Company:   o.Company,
		Location:  o.Location,
		Website:   o.WebsiteURL,
		URL:       o.URL,
		AvatarURL: o.AvatarURL,
		Followers: o.Followers.TotalCount,
		Following: o.Following.TotalCount,
		Repos:     o.Repositories.TotalCount,
		CreatedAt: o.CreatedAt,
	}
	if o.Status != nil {
		p.Status = core.Status{Emoji: o.Status.Emoji, Message: o.Status.Message, Busy: o.Status.Busy}
	}
	out := core.Owner{
		ID:         o.ID,
		Profile:    p,
		Pronouns:   o.Pronouns,
		Stars:      o.Starred.TotalCount,
		Twitter:    o.Twitter,
		Email:      o.Email,
		Verified:   o.Verified,
		Members:    o.Members.TotalCount,
		Teams:      o.Teams.TotalCount,
		Pinned:     ownerPinned(o.PinnedItems.Nodes),
		HiddenPins: slices.Contains(o.PinnedItems.Nodes, nil),
		Viewer: core.Relation{
			IsViewer:      o.IsViewer,
			Following:     o.ViewerFollows,
			CanFollow:     o.CanFollow,
			FollowsViewer: o.FollowsViewer,
			Member:        o.Member,
			CanAdminister: o.CanAdminister,
		},
	}
	if o.Typename == "Organization" {
		out.Kind = core.OwnerOrg
		out.Profile.Bio = o.Description
	}
	return out
}

// ownerPinned converts the pinned repositories, leaving out those GitHub
// hid from the token, which come as nulls.
func ownerPinned(in []*repoNode) []core.Repo {
	var out []core.Repo
	for _, r := range in {
		if r != nil {
			out = append(out, r.core())
		}
	}
	return out
}

// OwnerHeader returns the profile, the counts, the pinned repositories and
// the viewer's relation of the user or organization login. It returns an
// error matching core.ErrNotFound if there is no such account. If GitHub
// answers only part of the query, such as when a pinned repository is
// hidden from the token, it returns what GitHub answered along with the
// error.
func (c *Client) OwnerHeader(ctx context.Context, login string) (core.Owner, error) {
	var data struct {
		Owner *ownerNode `json:"repositoryOwner"`
	}
	vars := map[string]any{"login": login, "pinned": maxPinned}
	err := c.Query(ctx, ownerHeaderQuery, vars, &data)
	if data.Owner == nil {
		if err == nil {
			err = core.ErrNotFound
		}
		return core.Owner{}, fmt.Errorf("owner %s: %w", login, err)
	}
	if err != nil {
		return data.Owner.core(), fmt.Errorf("owner %s: %w", login, err)
	}
	return data.Owner.core(), nil
}

// userReposQuery lists the repositories a user owns, in the order asked.
const userReposQuery = `query UserRepos($login: String!, $first: Int!, $after: String, $field: RepositoryOrderField!, $direction: OrderDirection!) {
  ` + rateLimitField + `
  user(login: $login) {
    repositories(
      first: $first
      after: $after
      orderBy: {field: $field, direction: $direction}
      ownerAffiliations: [OWNER]
    ) {
      nodes { ...repoFields }
      pageInfo { hasNextPage endCursor }
    }
  }
}
` + repoFields

// repoOrderFields are GitHub's names of the fields repositories sort by.
var repoOrderFields = map[core.RepoOrderField]string{
	core.RepoOrderUpdated: "UPDATED_AT",
	core.RepoOrderPushed:  "PUSHED_AT",
	core.RepoOrderCreated: "CREATED_AT",
	core.RepoOrderName:    "NAME",
	core.RepoOrderStars:   "STARGAZERS",
}

// UserRepos returns a page of up to first repositories that the user login
// owns and the viewer can see, in order. After is the Next of the previous
// page, or empty for the first page. It returns an error matching
// core.ErrNotFound if there is no such user.
func (c *Client) UserRepos(ctx context.Context, login string, order core.RepoOrder, first int, after string) (core.Page[core.Repo], error) {
	field, ok := repoOrderFields[order.Field]
	if !ok {
		return core.Page[core.Repo]{}, fmt.Errorf("list repos of %s: unknown order field %d", login, order.Field)
	}
	direction := "DESC"
	if order.Ascending {
		direction = "ASC"
	}
	var data struct {
		User *struct {
			Repositories ownerRepos `json:"repositories"`
		} `json:"user"`
	}
	vars := ownerReposVars(first, after)
	vars["login"], vars["field"], vars["direction"] = login, field, direction
	if err := c.Query(ctx, userReposQuery, vars, &data); err != nil {
		return core.Page[core.Repo]{}, fmt.Errorf("list repos of %s: %w", login, err)
	}
	if data.User == nil {
		return core.Page[core.Repo]{}, fmt.Errorf("list repos of %s: %w", login, core.ErrNotFound)
	}
	return data.User.Repositories.core(), nil
}
