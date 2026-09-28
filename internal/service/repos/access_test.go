package repos

import (
	"errors"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/access"
)

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

// withCaps has s hold ghTUI with caps, as Get would have read them.
func withCaps(s *Service, caps core.RepoCaps) {
	s.repos.Mutate(repoKey(ghTUI.Ref), func(r core.Repo) core.Repo {
		r.Caps = caps
		return r
	})
}

func TestStarRefused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access core.Access
		caps   core.RepoCaps
	}{
		{"no scope", classic("gist"), core.RepoCaps{}},
		{"public_repo in a private repository", classic("public_repo"), core.RepoCaps{Known: true, Private: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{t: t}
			s, items := seeded(t, api)
			s.access = tokenWith(tc.access, true)
			withCaps(s, tc.caps)
			detail, _ := s.CachedGet(ghTUI.Ref)

			err := s.Star(ghTUI.Ref).Do(t.Context())
			if _, ok := errors.AsType[*core.ScopeError](err); !ok {
				t.Fatalf("Do = %v, want a ScopeError", err)
			}
			if p := core.Explain("star", err); p.Kind != core.Auth || p.Grant != "repo" {
				t.Errorf("Explain = %+v, want Auth granting repo", p)
			}
			wantCached(t, s, items, detail)
			if calls := api.Calls(); len(calls) != 0 {
				t.Errorf("calls = %q, want none", calls)
			}
		})
	}
}

func TestStarAllowed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access *access.Service
		caps   core.RepoCaps
	}{
		{"unknown token", tokenWith(core.Access{}, true), core.RepoCaps{}},
		{"repo", tokenWith(classic("repo"), true), core.RepoCaps{Known: true, Private: true}},
		{"public_repo in a public repository", tokenWith(classic("public_repo"), true), core.RepoCaps{Known: true}},
		// A repository not read yet may be public, where public_repo is
		// enough.
		{"public_repo, privacy unknown", tokenWith(classic("public_repo"), true), core.RepoCaps{}},
		{"checks off", tokenWith(classic("gist"), false), core.RepoCaps{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{t: t, star: func(core.RepoRef, bool) error { return nil }}
			s, _ := seeded(t, api)
			s.access = tc.access
			withCaps(s, tc.caps)
			if err := s.Star(ghTUI.Ref).Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if calls := api.Calls(); len(calls) != 1 {
				t.Errorf("calls = %q, want the star", calls)
			}
		})
	}
}
