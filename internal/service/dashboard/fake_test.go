package dashboard

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
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

	header        func() (core.Header, error)
	work          func(first int) (core.Work, error)
	contributions func() (core.Contributions, error)
	ownRepos      func(first int, after string) (core.Page[core.Repo], error)
	orgRepos      func(login string, first int, after string) (core.Page[core.Repo], error)

	mu    sync.Mutex
	calls []string
}

func (f *fakeAPI) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

// Calls returns the calls made so far, such as "header" or "org o 100 c1".
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

func (f *fakeAPI) ViewerHeader(context.Context) (core.Header, error) {
	f.record("header")
	if f.header == nil {
		f.t.Error("unexpected ViewerHeader")
		return core.Header{}, errUnexpected
	}
	return f.header()
}

func (f *fakeAPI) ViewerWork(_ context.Context, first int) (core.Work, error) {
	f.record(fmt.Sprintf("work %d", first))
	if f.work == nil {
		f.t.Error("unexpected ViewerWork")
		return core.Work{}, errUnexpected
	}
	return f.work(first)
}

func (f *fakeAPI) ViewerContributions(context.Context) (core.Contributions, error) {
	f.record("contributions")
	if f.contributions == nil {
		f.t.Error("unexpected ViewerContributions")
		return core.Contributions{}, errUnexpected
	}
	return f.contributions()
}

func (f *fakeAPI) ViewerOwnRepos(_ context.Context, first int, after string) (core.Page[core.Repo], error) {
	f.record(fmt.Sprintf("own %d %s", first, after))
	if f.ownRepos == nil {
		f.t.Error("unexpected ViewerOwnRepos")
		return core.Page[core.Repo]{}, errUnexpected
	}
	return f.ownRepos(first, after)
}

func (f *fakeAPI) OrgRepos(_ context.Context, login string, first int, after string) (core.Page[core.Repo], error) {
	f.record(fmt.Sprintf("org %s %d %s", login, first, after))
	if f.orgRepos == nil {
		f.t.Error("unexpected OrgRepos")
		return core.Page[core.Repo]{}, errUnexpected
	}
	return f.orgRepos(login, first, after)
}

var octocat = core.Header{
	Profile: core.Profile{Login: "octocat", Name: "The Octocat", Followers: 24214},
	Pinned:  []core.Repo{{Ref: core.RepoRef{Owner: "octocat", Name: "Spoon-Knife"}}},
	Orgs:    []core.Org{{Login: "github", Name: "GitHub"}},
}

// repos returns n repositories of owner named from the first'th on.
func repos(owner string, first, n int) []core.Repo {
	rs := make([]core.Repo, n)
	for i := range n {
		rs[i] = core.Repo{Ref: core.RepoRef{Owner: owner, Name: fmt.Sprintf("repo%d", first+i)}}
	}
	return rs
}

// pagedOrg answers OrgRepos for an organization of total repositories, in
// pages whose cursors are the index of their first repository.
func pagedOrg(total int) func(login string, first int, after string) (core.Page[core.Repo], error) {
	return func(login string, first int, after string) (core.Page[core.Repo], error) {
		from := 0
		if after != "" {
			var err error
			if from, err = strconv.Atoi(after); err != nil {
				return core.Page[core.Repo]{}, err
			}
		}
		n := min(first, total-from)
		p := core.Page[core.Repo]{Items: repos(login, from, n)}
		if from+n < total {
			p.Next = strconv.Itoa(from + n)
		}
		return p, nil
	}
}
