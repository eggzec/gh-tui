package owners

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var _ API = (*github.Client)(nil)

// fakeAPI answers with its func fields and records the calls. A nil field
// fails the test when it is called.
type fakeAPI struct {
	t testing.TB

	header        func(login string) (core.Owner, error)
	userRepos     func(login string, order core.RepoOrder, first int, after string) (core.Page[core.Repo], error)
	orgRepos      func(login string, first int, after string) (core.Page[core.Repo], error)
	contributions func(login string) (core.Contributions, error)
	stars         func(login string, first int, after string) (core.Page[core.Repo], error)
	// people answers the lists of people, by the name of the list, such
	// as "followers".
	people       func(list, login string, first int, after string) (core.Page[core.Person], error)
	teams        func(login string, first int, after string) (core.Page[core.Team], error)
	readme       func(login string, kind core.OwnerKind, member bool, cond github.Conditional) (core.Readme, github.Response, error)
	orgFollowers func(login string) (int, error)

	mu    sync.Mutex
	calls []string
}

func (f *fakeAPI) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

// Calls returns the calls made so far, such as "header octocat".
func (f *fakeAPI) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeAPI) wantCalls(t *testing.T, want ...string) {
	t.Helper()
	if got := f.Calls(); !slices.Equal(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

var errUnexpected = errors.New("unexpected call")

func (f *fakeAPI) OwnerHeader(_ context.Context, login string) (core.Owner, error) {
	f.record("header " + login)
	if f.header == nil {
		f.t.Error("unexpected OwnerHeader")
		return core.Owner{}, errUnexpected
	}
	return f.header(login)
}

func (f *fakeAPI) UserRepos(_ context.Context, login string, order core.RepoOrder, first int, after string) (core.Page[core.Repo], error) {
	f.record(fmt.Sprintf("user %s %d %s", login, first, after))
	if f.userRepos == nil {
		f.t.Error("unexpected UserRepos")
		return core.Page[core.Repo]{}, errUnexpected
	}
	return f.userRepos(login, order, first, after)
}

func (f *fakeAPI) OrgRepos(_ context.Context, login string, first int, after string) (core.Page[core.Repo], error) {
	f.record(fmt.Sprintf("org %s %d %s", login, first, after))
	if f.orgRepos == nil {
		f.t.Error("unexpected OrgRepos")
		return core.Page[core.Repo]{}, errUnexpected
	}
	return f.orgRepos(login, first, after)
}

func (f *fakeAPI) UserContributions(_ context.Context, login string) (core.Contributions, error) {
	f.record("contributions " + login)
	if f.contributions == nil {
		f.t.Error("unexpected UserContributions")
		return core.Contributions{}, errUnexpected
	}
	return f.contributions(login)
}

func (f *fakeAPI) UserStars(_ context.Context, login string, first int, after string) (core.Page[core.Repo], error) {
	f.record(fmt.Sprintf("stars %s %d %s", login, first, after))
	if f.stars == nil {
		f.t.Error("unexpected UserStars")
		return core.Page[core.Repo]{}, errUnexpected
	}
	return f.stars(login, first, after)
}

func (f *fakeAPI) listPeople(list, login string, first int, after string) (core.Page[core.Person], error) {
	f.record(fmt.Sprintf("%s %s %d %s", list, login, first, after))
	if f.people == nil {
		f.t.Errorf("unexpected list of %s", list)
		return core.Page[core.Person]{}, errUnexpected
	}
	return f.people(list, login, first, after)
}

func (f *fakeAPI) UserFollowers(_ context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	return f.listPeople("followers", login, first, after)
}

func (f *fakeAPI) UserFollowing(_ context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	return f.listPeople("following", login, first, after)
}

func (f *fakeAPI) UserOrgs(_ context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	return f.listPeople("orgs", login, first, after)
}

func (f *fakeAPI) OrgMembers(_ context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	return f.listPeople("members", login, first, after)
}

func (f *fakeAPI) OwnerSponsors(_ context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	return f.listPeople("sponsors", login, first, after)
}

func (f *fakeAPI) OwnerSponsoring(_ context.Context, login string, first int, after string) (core.Page[core.Person], error) {
	return f.listPeople("sponsoring", login, first, after)
}

func (f *fakeAPI) OrgTeams(_ context.Context, login string, first int, after string) (core.Page[core.Team], error) {
	f.record(fmt.Sprintf("teams %s %d %s", login, first, after))
	if f.teams == nil {
		f.t.Error("unexpected OrgTeams")
		return core.Page[core.Team]{}, errUnexpected
	}
	return f.teams(login, first, after)
}

// ProfileReadme records the call as "readme login member etag".
func (f *fakeAPI) ProfileReadme(_ context.Context, login string, kind core.OwnerKind, member bool, cond github.Conditional) (core.Readme, github.Response, error) {
	f.record(fmt.Sprintf("readme %s %t %s", login, member, cond.ETag))
	if f.readme == nil {
		f.t.Error("unexpected ProfileReadme")
		return core.Readme{}, github.Response{}, errUnexpected
	}
	return f.readme(login, kind, member, cond)
}

func (f *fakeAPI) OrgFollowers(_ context.Context, login string) (int, error) {
	f.record("orgfollowers " + login)
	if f.orgFollowers == nil {
		f.t.Error("unexpected OrgFollowers")
		return 0, errUnexpected
	}
	return f.orgFollowers(login)
}

var octocat = core.Owner{
	Kind:    core.OwnerUser,
	ID:      "U_1",
	Profile: core.Profile{Login: "octocat", Name: "The Octocat"},
	Pinned:  []core.Repo{{Ref: core.RepoRef{Owner: "octocat", Name: "Spoon-Knife"}}},
}

// repos returns n repositories of owner.
func repos(owner string, n int) []core.Repo {
	rs := make([]core.Repo, n)
	for i := range n {
		rs[i] = core.Repo{Ref: core.RepoRef{Owner: owner, Name: fmt.Sprintf("repo%d", i)}}
	}
	return rs
}

// people returns n accounts, named after prefix.
func people(prefix string, n int) []core.Person {
	ps := make([]core.Person, n)
	for i := range n {
		ps[i] = core.Person{Login: fmt.Sprintf("%s%d", prefix, i)}
	}
	return ps
}
