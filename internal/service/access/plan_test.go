package access

import (
	"reflect"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestPlan(t *testing.T) {
	const (
		gh     = "/usr/bin/gh"
		tokens = "https://github.com/settings/tokens"
	)
	workflow := []string{"workflow"}
	tests := []struct {
		name string
		st   situation
		want Plan
	}{
		{
			name: "not known yet",
			st:   situation{kind: core.TokenClassic, source: "gh", gh: gh, missing: workflow},
			want: Plan{Why: "GitHub hasn't said yet what the token may do."},
		},
		{
			name: "nothing missing",
			st:   situation{kind: core.TokenClassic, known: true, source: "gh", gh: gh},
			want: Plan{Why: "The token may do everything gh-tui does."},
		},
		{
			name: "keyring",
			st:   situation{kind: core.TokenClassic, known: true, source: "gh", gh: gh, missing: workflow},
			want: Plan{
				Cmd: []string{gh, "auth", "refresh", "--hostname=github.com", "-s", "workflow"},
				Why: "gh opens the browser to grant the token the workflow scope.", Scopes: workflow,
			},
		},
		{
			name: "hosts file keeps its storage",
			st:   situation{kind: core.TokenClassic, known: true, source: "oauth_token", gh: gh, missing: []string{"notifications", "workflow"}},
			want: Plan{
				Cmd: []string{gh, "auth", "refresh", "--hostname=github.com", "-s", "notifications,workflow", "--insecure-storage"},
				Why: "gh opens the browser to grant the token the notifications and workflow scopes.", Scopes: []string{"notifications", "workflow"},
			},
		},
		{
			name: "enterprise",
			st:   situation{host: "ghe.corp", kind: core.TokenClassic, known: true, source: "gh", gh: gh, missing: []string{"a", "b", "c"}},
			want: Plan{
				Cmd: []string{gh, "auth", "refresh", "--hostname=ghe.corp", "-s", "a,b,c"},
				Why: "gh opens the browser to grant the token the a, b and c scopes.", Scopes: []string{"a", "b", "c"},
			},
		},
		{
			name: "sso only",
			st:   situation{kind: core.TokenClassic, known: true, source: "gh", gh: gh, sso: true},
			want: Plan{
				Cmd: []string{gh, "auth", "refresh", "--hostname=github.com"},
				Why: "gh signs in again, where you can authorize the organizations' SSO.",
			},
		},
		{
			name: "gh missing",
			st:   situation{kind: core.TokenClassic, known: true, source: "oauth_token", missing: workflow},
			want: Plan{Why: "Install GitHub CLI to grant the token the workflow scope.", Scopes: workflow},
		},
		{
			name: "gh missing, sso",
			st:   situation{kind: core.TokenClassic, known: true, source: "oauth_token", sso: true},
			want: Plan{Why: "Install GitHub CLI and run gh auth refresh to authorize the organizations' SSO."},
		},
		{
			name: "personal access token in gh",
			st:   situation{kind: core.TokenClassic, known: true, pat: true, source: "gh", gh: gh, missing: workflow},
			want: Plan{URL: tokens, Why: "Give the token the workflow scope on GitHub, then check again.", Scopes: workflow},
		},
		{
			name: "personal access token in gh, sso",
			st:   situation{kind: core.TokenClassic, known: true, pat: true, source: "oauth_token", gh: gh, sso: true},
			want: Plan{URL: tokens, Why: "Authorize the token for the organizations' SSO on GitHub, then check again."},
		},
		{
			name: "GH_TOKEN",
			st:   situation{kind: core.TokenClassic, known: true, source: "GH_TOKEN", missing: workflow},
			want: Plan{URL: tokens, Why: "The token comes from GH_TOKEN: update GH_TOKEN to a token with the workflow scope, or unset it and run gh auth login.", Scopes: workflow},
		},
		{
			name: "GITHUB_ENTERPRISE_TOKEN, sso",
			st:   situation{host: "ghe.corp", kind: core.TokenClassic, known: true, source: "GITHUB_ENTERPRISE_TOKEN", sso: true},
			want: Plan{URL: "https://ghe.corp/settings/tokens", Why: "The token comes from GITHUB_ENTERPRISE_TOKEN: authorize it for the organizations' SSO on GitHub, or unset GITHUB_ENTERPRISE_TOKEN and run gh auth login."},
		},
		{
			name: "given without a source",
			st:   situation{kind: core.TokenClassic, known: true, missing: workflow},
			want: Plan{URL: tokens, Why: "Give the token the workflow scope on GitHub, then check again.", Scopes: workflow},
		},
		{
			name: "fine-grained in gh",
			st:   situation{kind: core.TokenFineGrained, known: true, source: "gh", gh: gh, refused: true},
			want: Plan{Why: "A fine-grained token has no scopes to grant; run gh auth login to use a classic token."},
		},
		{
			name: "app token from GITHUB_TOKEN",
			st:   situation{kind: core.TokenApp, known: true, source: "GITHUB_TOKEN", refused: true},
			want: Plan{Why: "A GitHub App token has no scopes to grant; unset GITHUB_TOKEN and run gh auth login to use a classic token."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.st.host == "" {
				tt.st.host = "github.com"
			}
			if got := plan(tt.st); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("plan =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

// Refresh asks for the scope each need misses, once, and finds gh only
// for a token gh stores.
func TestRefresh(t *testing.T) {
	looked := 0
	s, _ := bound(Token{Value: "gho_x", Source: "gh"}, withGHPath(func() string { looked++; return "gh" }))
	s.Set(classic("gist", "read:org"))
	got := s.Refresh(core.NeedRuns, core.NeedWrite(core.RepoCaps{}), core.NeedNotifications, core.NeedWorkflow)
	want := []string{"gh", "auth", "refresh", "--hostname=github.com", "-s", "notifications,repo,workflow"}
	if !reflect.DeepEqual(got.Cmd, want) || looked != 1 {
		t.Errorf("Refresh = %q after %d lookups of gh, want %q after 1", got.Cmd, looked, want)
	}

	s, _ = bound(Token{Value: "ghp_x", Source: "GH_TOKEN"}, withGHPath(func() string { looked++; return "gh" }))
	s.Set(classic("repo"))
	if got := s.Refresh(core.NeedRuns, core.NeedWorkflow); got.Cmd != nil || looked != 1 {
		t.Errorf("Refresh of a token from GH_TOKEN = %+v, looked for gh %d times; want no command and no lookup", got, looked-1)
	}

	s, _ = bound(Token{Value: "github_pat_x", Source: "gh"}, withGHPath(func() string { return "gh" }))
	s.Set(core.Access{Kind: core.TokenFineGrained})
	if got := s.Refresh(core.NeedNotifications); got.Cmd != nil || got.Why == "" {
		t.Errorf("Refresh of a fine-grained token = %+v, want only why", got)
	}
}

// A host that could be read as more than a host, such as a flag, goes
// into no command and no link.
func TestPlanHost(t *testing.T) {
	why := "The host isn't a plain host name, so gh-tui can't refresh its token; run gh auth refresh yourself."
	for _, tt := range []struct {
		host string
		ok   bool
	}{
		{"github.com", true},
		{"ghe.corp:8443", true},
		{"GHE-1.example.com", true},
		{"10.0.0.1:443", true},
		{"--help", false},
		{"-h", false},
		{"ghe.corp/evil", false},
		{"ghe.corp:", false},
		{"ghe.corp:44a", false},
		{"ghe corp", false},
		{"", false},
		{"ghe.corp\x1b[2J", false},
	} {
		st := situation{host: tt.host, kind: core.TokenClassic, known: true, source: "gh", gh: "/usr/bin/gh", missing: []string{"workflow"}}
		got := plan(st)
		if tt.ok {
			want := []string{"/usr/bin/gh", "auth", "refresh", "--hostname=" + tt.host, "-s", "workflow"}
			if !reflect.DeepEqual(got.Cmd, want) {
				t.Errorf("plan for %q = %q, want %q", tt.host, got.Cmd, want)
			}
			continue
		}
		if got.Cmd != nil || got.URL != "" || got.Why != why {
			t.Errorf("plan for %q = %+v, want no command, no link and why", tt.host, got)
		}
		// The token from GH_TOKEN gets no link either.
		st.source = "GH_TOKEN"
		if got := plan(st); got.URL != "" {
			t.Errorf("plan for %q from GH_TOKEN links %q", tt.host, got.URL)
		}
	}
}
