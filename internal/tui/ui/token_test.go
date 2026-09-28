package ui

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// fakeChecker is what the access service knows of a token, checked the
// way the service checks it.
type fakeChecker struct {
	a   core.Access
	off bool
}

func (f fakeChecker) Access() core.Access { return f.a }

func (f fakeChecker) Check(n core.Need) error {
	if f.off {
		return nil
	}
	if ok, known := f.a.Allows(n); ok || !known {
		return nil
	}
	if f.a.Missing(n) == "" {
		return &core.KindError{Kind: f.a.Kind}
	}
	return &core.ScopeError{Scopes: slices.Clone(n.Scopes)}
}

func classic(scopes ...string) core.Access {
	return core.Access{Kind: core.TokenClassic, Known: true, Scopes: scopes}
}

func TestGateToken(t *testing.T) {
	repo := core.RepoRef{Owner: "eggzec", Name: "x"}
	admin := func(private bool) core.RepoCaps {
		return core.RepoCaps{Known: true, Private: private, Permission: core.PermissionAdmin, Issues: true, MergeCommit: true}
	}
	archived := admin(false)
	archived.Archived = true
	keys := map[string][]string{config.ActionCommand: {":"}}
	fine := core.Access{Kind: core.TokenFineGrained}

	tests := []struct {
		name   string
		access core.Access
		caps   core.RepoCaps
		off    bool
		keys   map[string][]string
		action Action
		why    string
	}{
		{name: "an unknown token merges", action: ActMerge, caps: admin(true)},
		{name: "scopes GitHub hasn't listed merge", access: core.Access{Kind: core.TokenClassic}, action: ActMerge, caps: admin(true)},
		{name: "repo merges in a private repository", access: classic("repo"), action: ActMerge, caps: admin(true)},
		{
			name: "public_repo can't merge in a private repository", access: classic("public_repo"), action: ActMerge, caps: admin(true),
			why: "Merging needs the repo scope · :auth to grant it",
		},
		{name: "public_repo merges in a public repository", access: classic("public_repo"), action: ActMerge, caps: admin(false)},
		{name: "public_repo may try where privacy isn't known", access: classic("public_repo"), action: ActMerge},
		{
			name: "no scope can't close", access: classic("gist"), action: ActClose, caps: admin(false),
			why: "Closing needs the public_repo scope · :auth to grant it",
		},
		{
			name: "no scope can't reopen", access: classic(), action: ActReopen, caps: admin(false),
			why: "Reopening needs the public_repo scope · :auth to grant it",
		},
		{
			name: "no scope can't change a draft", access: classic(), action: ActDraft,
			why: "Changing a draft needs the repo scope · :auth to grant it",
		},
		{
			name: "no scope can't comment", access: classic(), action: ActComment,
			why: "Commenting needs the repo scope · :auth to grant it",
		},
		{
			name: "no scope can't label", access: classic(), action: ActLabel,
			why: "Labeling needs the repo scope · :auth to grant it",
		},
		{
			name: "public_repo can't re-run", access: classic("public_repo"), action: ActRerun, caps: admin(false),
			why: "Re-running needs the repo scope · :auth to grant it",
		},
		{
			name: "public_repo can't cancel", access: classic("public_repo"), action: ActCancelRun, caps: admin(false),
			why: "Cancelling needs the repo scope · :auth to grant it",
		},
		{name: "repo re-runs", access: classic("repo"), action: ActRerun, caps: admin(false)},
		{name: "notifications mark read", access: classic("notifications"), action: ActMarkRead},
		{name: "repo marks read", access: classic("repo"), action: ActMarkRead},
		{
			name: "no scope can't mark read", access: classic("gist"), action: ActMarkRead,
			why: "Marking notifications needs the notifications scope · :auth to grant it",
		},
		{
			name: "a fine-grained token can't mark read", access: fine, action: ActMarkRead,
			why: "Marking notifications needs a classic token, not a fine-grained one · :auth to see how",
		},
		{name: "a fine-grained token may try a merge", access: fine, action: ActMerge, caps: admin(true)},
		{
			name: "the repository says more than the token", access: classic(), action: ActComment, caps: archived,
			why: "eggzec/x is archived, so it's read-only.",
		},
		{name: "checks turned off allow everything", access: classic(), off: true, action: ActMerge, caps: admin(true)},
		{
			name: "without a command key the hint is left out", access: classic(), keys: map[string][]string{}, action: ActMerge,
			caps: admin(true), why: "Merging needs the repo scope.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := keys
			if tt.keys != nil {
				k = tt.keys
			}
			g := Gate{Repo: repo, Caps: tt.caps, Token: NewToken(fakeChecker{a: tt.access, off: tt.off}, k)}
			ok, why := g.Allow(tt.action, nil)
			if ok != (tt.why == "") || why != tt.why {
				t.Errorf("Allow = %v, %q; want %v, %q", ok, why, tt.why == "", tt.why)
			}
		})
	}
}

func TestNilToken(t *testing.T) {
	var tok *Token
	if err := tok.Check(core.NeedNotifications); err != nil {
		t.Errorf("Check = %v, want nil", err)
	}
	if a := tok.Access(); !a.Equal(core.Access{}) {
		t.Errorf("Access = %+v, want the zero Access", a)
	}
	if h := tok.Hint(); h != "" {
		t.Errorf("Hint = %q, want none", h)
	}
}
