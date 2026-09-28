package core

import (
	"slices"
	"testing"
)

// TestCovers checks the hierarchy against GitHub's list of scopes, where a
// scope's children are indented below it.
func TestCovers(t *testing.T) {
	tests := []struct {
		have, want string
		ok         bool
	}{
		{"repo", "repo", true},
		{"repo", "repo:status", true},
		{"repo", "repo_deployment", true},
		{"repo", "public_repo", true},
		{"repo", "repo:invite", true},
		{"repo", "security_events", true},
		{"repo", "admin:repo_hook", true},
		{"repo", "write:repo_hook", true},
		{"repo", "read:repo_hook", true},
		{"repo", "workflow", false},
		{"repo", "notifications", false},
		{"repo", "read:org", false},
		{"repo", "gist", false},
		{"public_repo", "repo", false},
		{"public_repo", "repo:status", false},
		{"repo:status", "repo", false},
		{"admin:repo_hook", "write:repo_hook", true},
		{"admin:repo_hook", "read:repo_hook", true},
		{"write:repo_hook", "read:repo_hook", true},
		{"read:repo_hook", "write:repo_hook", false},
		{"admin:org", "write:org", true},
		{"admin:org", "read:org", true},
		{"write:org", "read:org", true},
		{"read:org", "write:org", false},
		{"write:org", "admin:org", false},
		{"admin:public_key", "read:public_key", true},
		{"admin:gpg_key", "write:gpg_key", true},
		{"admin:ssh_signing_key", "read:ssh_signing_key", true},
		{"admin:org", "admin:org_hook", false},
		{"user", "read:user", true},
		{"user", "user:email", true},
		{"user", "user:follow", true},
		{"read:user", "user", false},
		{"project", "read:project", true},
		{"write:packages", "read:packages", true},
		{"write:packages", "delete:packages", false},
		{"write:discussion", "read:discussion", true},
		{"admin:enterprise", "manage_runners:enterprise", true},
		{"admin:enterprise", "manage_billing:enterprise", true},
		{"admin:enterprise", "read:enterprise", true},
		{"workflow", "repo", false},
		{"notifications", "repo", false},
	}
	for _, tt := range tests {
		if got := Covers(tt.have, tt.want); got != tt.ok {
			t.Errorf("Covers(%q, %q) = %v, want %v", tt.have, tt.want, got, tt.ok)
		}
	}
}

// TestCoversNormalizes checks Covers against the examples of GitHub's
// docs of what a token is granted when it asks for scopes that include
// others: those left out are the ones another covers.
func TestCoversNormalizes(t *testing.T) {
	tests := []struct {
		asked, granted string
	}{
		{"user, gist, user:email", "gist, user"},
		{"repo, public_repo, repo:status", "repo"},
		{"admin:org, read:org", "admin:org"},
		{"write:org, read:org, gist", "gist, write:org"},
		{"repo, workflow", "repo, workflow"},
		{"notifications, repo", "notifications, repo"},
	}
	for _, tt := range tests {
		asked := ParseScopes(tt.asked)
		kept := slices.DeleteFunc(slices.Clone(asked), func(s string) bool {
			return slices.ContainsFunc(asked, func(o string) bool { return o != s && Covers(o, s) })
		})
		if want := ParseScopes(tt.granted); !slices.Equal(kept, want) {
			t.Errorf("asking for %q grants %v, want %v", tt.asked, kept, want)
		}
	}
}

func TestParseScopes(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{" , ", nil},
		{"gist, read:org, repo", []string{"gist", "read:org", "repo"}},
		{"repo,gist,repo", []string{"gist", "repo"}},
		{"  workflow  ,admin:repo_hook", []string{"admin:repo_hook", "workflow"}},
		{"repo, rm -rf, Repo, a;b, $(x)", []string{"repo"}},
	}
	for _, tt := range tests {
		if got := ParseScopes(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("ParseScopes(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSortScopes(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"notifications, repo", "notifications"},
		{"public_repo, repo", "public_repo"},
		{"repo, public_repo", "public_repo"},
		{"admin:org, read:org, repo, user, write:org", "read:org"},
		{"repo, workflow", "workflow"},
		{"repo", "repo"},
	}
	for _, tt := range tests {
		in := ParseScopes(tt.in)
		got := SortScopes(in)
		if len(got) != len(in) || got[0] != tt.want {
			t.Errorf("SortScopes(%q) = %q, want %q first", tt.in, got, tt.want)
		}
	}
}

func TestAllows(t *testing.T) {
	classic := func(scopes string) Access {
		return Access{Kind: TokenClassic, Known: true, Scopes: ParseScopes(scopes)}
	}
	gh := classic("gist, read:org, repo")
	private := RepoCaps{Known: true, Private: true}
	public := RepoCaps{Known: true}
	tests := []struct {
		name      string
		a         Access
		n         Need
		ok, known bool
		missing   string
	}{
		{"unknown token", Access{}, NeedNotifications, true, false, ""},
		{"unknown scopes", Access{Kind: TokenClassic}, NeedRuns, true, false, ""},
		{"needs nothing", classic(""), Need{}, true, true, ""},
		{"gh token notifications", gh, NeedNotifications, true, true, ""},
		{"gh token runs", gh, NeedRuns, true, true, ""},
		{"gh token workflow", gh, NeedWorkflow, false, true, "workflow"},
		{"gh token write private", gh, NeedWrite(private), true, true, ""},
		{"notifications scope", classic("notifications"), NeedNotifications, true, true, ""},
		{"no scopes notifications", classic(""), NeedNotifications, false, true, "notifications"},
		{"public_repo runs", classic("public_repo"), NeedRuns, false, true, "repo"},
		{"public_repo write public", classic("public_repo"), NeedWrite(public), true, true, ""},
		{"public_repo write private", classic("public_repo"), NeedWrite(private), false, true, "repo"},
		{"public_repo write unknown", classic("public_repo"), NeedWrite(RepoCaps{}), true, true, ""},
		{"no scopes write unknown", classic(""), NeedWrite(RepoCaps{}), false, true, "repo"},
		{"no scopes write public", classic("gist"), NeedWrite(public), false, true, "public_repo"},
		{"fine-grained notifications", Access{Kind: TokenFineGrained}, NeedNotifications, false, true, ""},
		{"app notifications", Access{Kind: TokenApp}, NeedNotifications, false, true, ""},
		{"fine-grained runs", Access{Kind: TokenFineGrained}, NeedRuns, true, false, ""},
		{"app write", Access{Kind: TokenApp}, NeedWrite(private), true, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, known := tt.a.Allows(tt.n)
			if ok != tt.ok || known != tt.known {
				t.Errorf("Allows = %v, %v, want %v, %v", ok, known, tt.ok, tt.known)
			}
			if got := tt.a.Missing(tt.n); got != tt.missing {
				t.Errorf("Missing = %q, want %q", got, tt.missing)
			}
		})
	}
}

func TestAccessEqual(t *testing.T) {
	a := Access{Kind: TokenClassic, Known: true, Scopes: []string{"repo"}, SSO: []string{"1"}}
	if !a.Equal(Access{Kind: TokenClassic, Known: true, Scopes: []string{"repo"}, SSO: []string{"1"}}) {
		t.Error("Equal of the same access = false")
	}
	for _, b := range []Access{
		{Kind: TokenClassic, Known: true, Scopes: []string{"repo"}},
		{Kind: TokenClassic, Scopes: []string{"repo"}, SSO: []string{"1"}},
		{Kind: TokenClassic, Known: true, Scopes: []string{"repo", "workflow"}, SSO: []string{"1"}},
		{Kind: TokenApp, Known: true, Scopes: []string{"repo"}, SSO: []string{"1"}},
	} {
		if a.Equal(b) {
			t.Errorf("Equal(%+v) = true", b)
		}
	}
}

func TestTokenKindString(t *testing.T) {
	for k := TokenUnknown; k <= TokenApp; k++ {
		if k != TokenUnknown && k.String() == TokenUnknown.String() {
			t.Errorf("%d has no name of its own", k)
		}
	}
}
