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
