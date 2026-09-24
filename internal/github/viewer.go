package github

import (
	"context"
	"fmt"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// maxViewerOrgs is how many organizations the header lists. Hardly anyone
// belongs to more, and the header is one request.
const maxViewerOrgs = 100

// maxPinned is how many items GitHub lets a profile pin.
const maxPinned = 6

// viewerHeaderQuery reads the profile, pins and organizations in one
// request. Pinned items may be gists too, so types keeps them to
// repositories.
const viewerHeaderQuery = `query ViewerHeader($pinned: Int!, $orgs: Int!) {
  ` + rateLimitField + `
  viewer {
    login
    name
    bio
    company
    location
    websiteUrl
    url
    avatarUrl
    createdAt
    followers { totalCount }
    following { totalCount }
    repositories(ownerAffiliations: [OWNER]) { totalCount }
    status { emoji message indicatesLimitedAvailability }
    pinnedItems(first: $pinned, types: [REPOSITORY]) {
      nodes { ...repoFields }
    }
    organizations(first: $orgs) {
      nodes { login name url }
    }
  }
}
` + repoFields

type viewerCount struct {
	TotalCount int `json:"totalCount"`
}

type viewerProfile struct {
	Login        string      `json:"login"`
	Name         string      `json:"name"`
	Bio          string      `json:"bio"`
	Company      string      `json:"company"`
	Location     string      `json:"location"`
	WebsiteURL   string      `json:"websiteUrl"`
	URL          string      `json:"url"`
	AvatarURL    string      `json:"avatarUrl"`
	CreatedAt    time.Time   `json:"createdAt"`
	Followers    viewerCount `json:"followers"`
	Following    viewerCount `json:"following"`
	Repositories viewerCount `json:"repositories"`
	Status       *struct {
		Emoji   string `json:"emoji"`
		Message string `json:"message"`
		Busy    bool   `json:"indicatesLimitedAvailability"`
	} `json:"status"`
	PinnedItems   nodes[repoNode]  `json:"pinnedItems"`
	Organizations nodes[viewerOrg] `json:"organizations"`
}

func (v *viewerProfile) core() core.Header {
	p := core.Profile{
		Login:     v.Login,
		Name:      v.Name,
		Bio:       v.Bio,
		Company:   v.Company,
		Location:  v.Location,
		Website:   v.WebsiteURL,
		URL:       v.URL,
		AvatarURL: v.AvatarURL,
		Followers: v.Followers.TotalCount,
		Following: v.Following.TotalCount,
		Repos:     v.Repositories.TotalCount,
		CreatedAt: v.CreatedAt,
	}
	if v.Status != nil {
		p.Status = core.Status{Emoji: v.Status.Emoji, Message: v.Status.Message, Busy: v.Status.Busy}
	}
	return core.Header{
		Profile: p,
		Pinned:  convert(v.PinnedItems.Nodes, repoNode.core),
		Orgs:    convert(v.Organizations.Nodes, viewerOrg.core),
	}
}

type viewerOrg struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

func (o viewerOrg) core() core.Org {
	return core.Org(o)
}

// ViewerHeader returns the signed-in user's profile, the repositories they
// pinned and up to 100 of the organizations they belong to. GitHub leaves
// out organizations that restrict what the token may see.
func (c *Client) ViewerHeader(ctx context.Context) (core.Header, error) {
	var data struct {
		Viewer viewerProfile `json:"viewer"`
	}
	vars := map[string]any{"pinned": maxPinned, "orgs": maxViewerOrgs}
	if err := c.Query(ctx, viewerHeaderQuery, vars, &data); err != nil {
		return core.Header{}, fmt.Errorf("viewer header: %w", err)
	}
	return data.Viewer.core(), nil
}
