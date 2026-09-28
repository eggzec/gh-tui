package pulls

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/access"
)

// fakeRepos holds the caps of the repositories read so far.
type fakeRepos map[core.RepoRef]core.RepoCaps

func (f fakeRepos) CachedGet(ref core.RepoRef) (core.Repo, bool) {
	c, ok := f[ref]
	return core.Repo{Ref: ref, Caps: c}, ok
}

// classic is a classic token that GitHub said has scopes.
func classic(scopes ...string) core.Access {
	return core.Access{Kind: core.TokenClassic, Known: true, Scopes: scopes}
}

// tokenWith returns the access service of a token that GitHub said a
// is, which refuses what a lacks unless checks is false.
func tokenWith(a core.Access, checks bool) *access.Service {
	s := access.New("github.com", access.Token{}, access.WithChecks(checks))
	s.Set(a)
	return s
}

// callsSoFar returns how often each method was called so far.
func (f *fakeAPI) callsSoFar() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.calls)
}

var (
	public  = fakeRepos{repo: {Known: true}}
	private = fakeRepos{repo: {Known: true, Private: true}}
)

func TestMutationRefused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access core.Access
		repos  fakeRepos
	}{
		{"no scope", classic("gist"), nil},
		{"public_repo in a private repository", classic("public_repo"), private},
	} {
		for _, m := range mutations {
			t.Run(tc.name+"/"+m.name, func(t *testing.T) {
				api := &fakeAPI{mutate: func(context.Context, string, string, core.MergeMethod) (core.PullRequest, error) {
					t.Error("a refused change was sent")
					return core.PullRequest{}, nil
				}}
				s := seeded(t, api, m.seed)
				s.access, s.repos = tokenWith(tc.access, true), tc.repos
				before, sent := take(t, s), api.callsSoFar()

				err := m.run(s, 1).Do(t.Context())
				if _, ok := errors.AsType[*core.ScopeError](err); !ok {
					t.Fatalf("Do = %v, want a ScopeError", err)
				}
				if p := core.Explain(m.name, err); p.Kind != core.Auth || p.Grant != "repo" {
					t.Errorf("Explain = %+v, want Auth granting repo", p)
				}
				if after := take(t, s); !reflect.DeepEqual(after, before) {
					t.Errorf("a refused change was shown:\n%+v\nwant\n%+v", after, before)
				}
				if got := api.callsSoFar(); !maps.Equal(got, sent) {
					t.Errorf("calls = %v, want %v: nothing sent", got, sent)
				}
			})
		}
	}
}

func TestMutationAllowed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access *access.Service
		repos  fakeRepos
	}{
		{"unknown token", tokenWith(core.Access{}, true), private},
		{"scopes not yet known", tokenWith(core.Access{Kind: core.TokenClassic}, true), private},
		{"fine-grained", tokenWith(core.Access{Kind: core.TokenFineGrained}, true), private},
		{"repo", tokenWith(classic("repo"), true), private},
		{"public_repo in a public repository", tokenWith(classic("public_repo"), true), public},
		// A repository not read yet may be public, where public_repo is
		// enough.
		{"public_repo, privacy unknown", tokenWith(classic("public_repo"), true), nil},
		{"checks off", tokenWith(classic("gist"), false), private},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{mutate: func(context.Context, string, string, core.MergeMethod) (core.PullRequest, error) {
				return core.PullRequest{Number: 1}, nil
			}}
			s := seeded(t, api, func(*core.PullRequest) {})
			s.access, s.repos = tc.access, tc.repos
			if err := s.Close(repo, 1).Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if n := api.count("close"); n != 1 {
				t.Errorf("close sent %d times, want 1", n)
			}
		})
	}
}
