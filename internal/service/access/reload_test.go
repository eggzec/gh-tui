package access

import (
	"errors"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestReload(t *testing.T) {
	start := Token{Value: "gho_old", Source: "gh", Login: "octocat"}
	tests := []struct {
		name    string
		found   Token
		tokens  []string
		probes  int
		wantErr error
	}{
		{name: "refreshed", found: Token{Value: "gho_new", Source: "gh", Login: "OctoCat"}, tokens: []string{"gho_new"}, probes: 1},
		{name: "unchanged", found: start, probes: 1},
		{name: "another login", found: Token{Value: "gho_new", Source: "gh", Login: "hubot"}, wantErr: ErrOtherAccount},
		{name: "no login", found: Token{Value: "ghp_new", Source: "GH_TOKEN"}, wantErr: ErrOtherAccount},
		{name: "gone", found: Token{Source: "default"}, wantErr: core.ErrUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, c := bound(start, WithLookup(func(host string) Token {
				if host != "github.com" {
					t.Errorf("looked up %q, want github.com", host)
				}
				return tt.found
			}))
			c.learned = classic("repo", "workflow")
			got, err := s.Reload(t.Context())
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil) != (err == nil) {
				t.Fatalf("Reload = %v, want %v", err, tt.wantErr)
			}
			tokens, probes := c.counts()
			if !slices.Equal(tokens, tt.tokens) || probes != tt.probes {
				t.Errorf("set tokens %q and probed %d times, want %q and %d", tokens, probes, tt.tokens, tt.probes)
			}
			if err == nil && !got.Equal(c.learned) {
				t.Errorf("Reload = %+v, want what the probe learned", got)
			}
			if err != nil && s.token != start {
				t.Errorf("the token is %+v after a failed reload, want the one it had", s.token)
			}
		})
	}
}

func TestReloadProbeFails(t *testing.T) {
	s, c := bound(Token{Value: "gho_old", Source: "gh", Login: "octocat"},
		WithLookup(func(string) Token { return Token{Value: "gho_new", Source: "gh", Login: "octocat"} }))
	c.err = core.ErrOffline
	if _, err := s.Reload(t.Context()); !errors.Is(err, core.ErrOffline) {
		t.Errorf("Reload = %v, want the probe's error", err)
	}
	// The token was read again, so it is the one to send.
	if tokens, _ := c.counts(); !slices.Equal(tokens, []string{"gho_new"}) {
		t.Errorf("set tokens %q, want the new one", tokens)
	}
}

func TestReloadUnbound(t *testing.T) {
	s := New("github.com", Token{Value: "gho_old"})
	if _, err := s.Reload(t.Context()); err == nil {
		t.Error("Reload without a client or a lookup succeeded")
	}
}

// A token found after the service was made is the one Reload compares
// the token it reads again with.
func TestReloadAfterFound(t *testing.T) {
	found := Token{Value: "gho_found", Source: "gh", Login: "octocat"}
	s, c := bound(Token{Source: "gh", Login: "octocat"}, WithLookup(func(string) Token { return found }))
	s.Found(found)
	if _, err := s.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	if tokens, _ := c.counts(); len(tokens) != 0 {
		t.Errorf("set tokens %q, want none: the token read again is the one found", tokens)
	}
}
