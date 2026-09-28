package issues

import (
	"errors"
	"reflect"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/access"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
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

var (
	public  = fakeRepos{repo: {Known: true}}
	private = fakeRepos{repo: {Known: true, Private: true}}
)

// changes are the changes to issue 7 that the token may be refused.
var changes = []struct {
	name string
	run  func(s *Service) *optimistic.Op
}{
	{"close", func(s *Service) *optimistic.Op { return s.Close(repo, 7) }},
	{"reopen", func(s *Service) *optimistic.Op { return s.Reopen(repo, 7) }},
	{"label", func(s *Service) *optimistic.Op { return s.AddLabels(repo, 7, []string{"enhancement"}) }},
	{"unlabel", func(s *Service) *optimistic.Op { return s.RemoveLabel(repo, 7, "enhancement") }},
	{"comment", func(s *Service) *optimistic.Op { return s.Comment(repo, 7, "Thanks!") }},
}

func TestChangeRefused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access core.Access
		repos  fakeRepos
	}{
		{"no scope", classic("gist"), nil},
		{"public_repo in a private repository", classic("public_repo"), private},
	} {
		for _, c := range changes {
			t.Run(tc.name+"/"+c.name, func(t *testing.T) {
				api := &fakeAPI{t: t}
				s := prime(t, api)
				s.access, s.repos = tokenWith(tc.access, true), tc.repos
				before := look(t, s)

				err := c.run(s).Do(t.Context())
				if _, ok := errors.AsType[*core.ScopeError](err); !ok {
					t.Fatalf("Do = %v, want a ScopeError", err)
				}
				if p := core.Explain(c.name, err); p.Kind != core.Auth || p.Grant != "repo" {
					t.Errorf("Explain = %+v, want Auth granting repo", p)
				}
				if after := look(t, s); !reflect.DeepEqual(after, before) {
					t.Errorf("a refused change was shown:\n%+v\nwant\n%+v", after, before)
				}
				api.checkCalls(t)
			})
		}
	}
}

func TestChangeAllowed(t *testing.T) {
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
			api := &fakeAPI{t: t}
			s := prime(t, api)
			s.access, s.repos = tc.access, tc.repos
			api.setState = func(number int, state core.State) (core.Issue, error) {
				it := issue(number)
				it.State = state
				return it, nil
			}
			if err := s.Close(repo, 7).Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			api.checkCalls(t, "SetIssueState")
		})
	}
}
