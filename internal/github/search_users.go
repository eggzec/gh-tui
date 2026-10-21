package github

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
)

// userSearchResults is the body of a user search response. The one endpoint
// finds users and organizations, told apart by the type: qualifier.
type userSearchResults struct {
	TotalCount        int             `json:"total_count"`
	IncompleteResults bool            `json:"incomplete_results"`
	Items             []userSearchHit `json:"items"`
}

// userSearchHit is the REST shape of an account in user search results.
type userSearchHit struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

func (h userSearchHit) core() core.SearchUser {
	kind := core.OwnerUser
	if h.Type == "Organization" {
		kind = core.OwnerOrg
	}
	return core.SearchUser{Login: h.Login, Kind: kind}
}

// SearchUsers returns a page of the accounts that match query, best match
// first. Query takes GitHub's user search syntax, so type:user or type:org
// narrows it to users or to organizations. Cursor, PerPage and the limit
// are as for SearchRepos; user searches share the search limit.
func (c *Client) SearchUsers(ctx context.Context, query, cursor string, perPage int) (core.SearchPage[core.SearchUser], error) {
	var body userSearchResults
	next, err := c.search(ctx, "search/users", query, cursor, perPage, &body)
	if err != nil {
		return core.SearchPage[core.SearchUser]{}, fmt.Errorf("search users: %w", err)
	}
	return core.SearchPage[core.SearchUser]{
		Items:      convert(body.Items, userSearchHit.core),
		Next:       next,
		Total:      body.TotalCount,
		Incomplete: body.IncompleteResults,
	}, nil
}
