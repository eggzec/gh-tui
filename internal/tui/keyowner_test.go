package tui

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	ownersvc "github.com/eggzec/gh-tui/internal/service/owners"
)

// keyOwnerSide serves the README and the calendar of the user keyOwners
// serves, for the keys of the panes beside the list.
type keyOwnerSide struct{}

func (keyOwnerSide) FreshReadme(ownersvc.ReadmeQuery) bool { return true }
func (keyOwnerSide) Readme(context.Context, ownersvc.ReadmeQuery) (ownersvc.Readme, error) {
	return ownersvc.Readme{Markdown: "# octocat", Source: core.RepoRef{Owner: "octocat", Name: "octocat"}}, nil
}
func (keyOwnerSide) FreshContributions(string) bool { return true }
func (keyOwnerSide) Contributions(context.Context, ownersvc.ContributionsQuery) (core.Contributions, error) {
	return core.Contributions{}, nil
}
func (keyOwnerSide) FreshOrgFollowers(string) bool { return true }
func (keyOwnerSide) OrgFollowers(context.Context, ownersvc.OrgFollowersQuery) (ownersvc.FollowerCount, error) {
	return ownersvc.FollowerCount{}, nil
}
func (keyOwnerSide) FreshPeople(ownersvc.PeopleQuery) bool { return true }
func (keyOwnerSide) People(context.Context, ownersvc.PeopleQuery) (core.Page[core.Person], error) {
	return core.Page[core.Person]{}, nil
}
