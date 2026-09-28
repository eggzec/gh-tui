package actions

import (
	"errors"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/access"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
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

// runOps are the changes to run 1 that the token may be refused.
var runOps = []struct {
	name string
	op   func(s *Service) *optimistic.Op
}{
	{"re-run", func(s *Service) *optimistic.Op { return s.RerunRun(repo, 1) }},
	{"re-run failed jobs", func(s *Service) *optimistic.Op { return s.RerunFailedJobs(repo, 1) }},
	{"re-run job", func(s *Service) *optimistic.Op { return s.RerunJob(repo, 1, 11) }},
	{"cancel", func(s *Service) *optimistic.Op { return s.CancelRun(repo, 1) }},
}

func TestRunOpRefused(t *testing.T) {
	// public_repo is enough to change an issue of a public repository,
	// but GitHub asks for repo to re-run or cancel a run in any.
	for _, o := range runOps {
		t.Run(o.name, func(t *testing.T) {
			f := newFake()
			s := primed(t, f)
			s.access = tokenWith(classic("public_repo"), true)

			err := o.op(s).Do(t.Context())
			if _, ok := errors.AsType[*core.ScopeError](err); !ok {
				t.Fatalf("Do = %v, want a ScopeError", err)
			}
			if p := core.Explain(o.name, err); p.Kind != core.Auth || p.Grant != "repo" {
				t.Errorf("Explain = %+v, want Auth granting repo", p)
			}
			checkShown(t, s, 1, completed, completed, completed)
			checkCalls(t, f)
		})
	}
}

func TestRunOpAllowed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access *access.Service
	}{
		{"unknown token", tokenWith(core.Access{}, true)},
		{"scopes not yet known", tokenWith(core.Access{Kind: core.TokenClassic}, true)},
		{"fine-grained", tokenWith(core.Access{Kind: core.TokenFineGrained}, true)},
		{"repo", tokenWith(classic("repo"), true)},
		{"checks off", tokenWith(classic("public_repo"), false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake()
			s := primed(t, f)
			s.access = tc.access
			if err := s.CancelRun(repo, 1).Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			checkCalls(t, f, "CancelRun octo-org/hello 1")
		})
	}
}
