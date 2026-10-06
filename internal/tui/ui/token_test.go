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
	keys := config.Keymap{config.ContextGlobal: {"command": {":"}}}
	fine := core.Access{Kind: core.TokenFineGrained}

	tests := []struct {
		name   string
		access core.Access
		caps   core.RepoCaps
		off    bool
		keys   config.Keymap
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
			name: "without a command key the hint is left out", access: classic(), keys: config.Keymap{}, action: ActMerge,
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

// TestSayToken checks the words for a token problem when the app has the
// command that grants the token what it lacks.
func TestSayToken(t *testing.T) {
	keys := config.Keymap{config.ContextGlobal: {"command": {":"}}}
	tok := func(a core.Access) *Token { return NewToken(fakeChecker{a: a}, keys) }
	tests := []struct {
		name        string
		p           *core.Problem
		access      core.Access
		text, hint  string
		toast       string
		narrowToast string
	}{
		{
			name:  "a missing scope",
			p:     core.Explain("merge #5", &core.ScopeError{Scopes: []string{"workflow"}}),
			text:  "The token lacks the workflow scope",
			hint:  ":auth to grant it",
			toast: "Couldn't merge #5: the token lacks the workflow scope · :auth to grant it.",
		},
		{
			name:        "a token of another kind",
			p:           core.Explain("mark read", &core.KindError{Kind: core.TokenFineGrained}),
			text:        "This needs a classic token, not a fine-grained one",
			hint:        ":auth to see how",
			toast:       "Couldn't mark read: this needs a classic token, not a fine-grained one · :auth to see how.",
			narrowToast: "Couldn't mark read: this needs a classic token, not a fine-grained one · :auth.",
		},
		{
			name: "an app's token",
			p:    core.Explain("load the notifications", &core.KindError{Kind: core.TokenApp}),
			text: "This needs a classic token, not a GitHub App's",
			hint: ":auth to see how",
		},
		{
			name:  "a rejected token",
			p:     core.Explain("load your profile", core.ErrUnauthorized),
			text:  "GitHub rejected the token. Run gh auth login, then :auth.",
			toast: "Couldn't load your profile: GitHub rejected the token. Run gh auth login, then :auth.",
		},
		{
			name:  "SSO",
			p:     &core.Problem{Kind: core.Forbidden, Action: "sync", Subject: "eggzec/x", SSO: true},
			text:  "eggzec requires SSO",
			hint:  ":auth to authorize the token",
			toast: "Couldn't sync: eggzec requires SSO · :auth to authorize the token.",
		},
		{
			name: "SSO of an unnamed organization",
			p:    &core.Problem{Kind: core.Forbidden, Action: "sync", SSO: true},
			text: "The organization requires SSO",
			hint: ":auth to authorize the token",
		},
		{
			name:   "not found without repo",
			p:      &core.Problem{Kind: core.NotFound, Action: "open eggzec/x", Subject: "eggzec/x"},
			access: classic("public_repo"),
			text:   "eggzec/x doesn't exist or is private. If it's private, the token needs the repo scope.",
			hint:   ":auth to grant it",
			toast:  "Couldn't open eggzec/x: eggzec/x doesn't exist or is private · :auth grants the repo scope.",
		},
		{
			name:   "not found with repo",
			p:      &core.Problem{Kind: core.NotFound, Action: "open eggzec/x", Subject: "eggzec/x"},
			access: classic("repo"),
			text:   "eggzec/x doesn't exist or is private.",
		},
		{
			name: "not found while the scopes aren't known",
			p:    &core.Problem{Kind: core.NotFound, Subject: "eggzec/x"},
			text: "eggzec/x doesn't exist or is private.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := testVoice()
			v.Token = tok(tt.access)
			text, hint := Say(tt.p, v)
			if text != tt.text || hint != tt.hint {
				t.Errorf("Say = %q, %q; want %q, %q", text, hint, tt.text, tt.hint)
			}
			if tt.toast != "" {
				if got := SayToast(tt.p, v, within(120)); got != tt.toast {
					t.Errorf("SayToast = %q, want %q", got, tt.toast)
				}
			}
			if tt.narrowToast != "" {
				if got := SayToast(tt.p, v, within(80)); got != tt.narrowToast {
					t.Errorf("SayToast in 80 cells = %q, want %q", got, tt.narrowToast)
				}
			}
		})
	}
}
