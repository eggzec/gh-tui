package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/cli/go-gh/v2/pkg/config"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestQuickToken(t *testing.T) {
	const octocat = "hosts:\n  github.com:\n    user: octocat\n"
	tests := []struct {
		name                 string
		stored, from, hosts  string
		token, source, login string
	}{
		{name: "hosts file", stored: "gho_file", from: sourceHostsFile, hosts: octocat, token: "gho_file", source: sourceHostsFile, login: "octocat"},
		{name: "environment", stored: "ghp_env", from: "GH_TOKEN", hosts: octocat, token: "ghp_env", source: "GH_TOKEN"},
		{name: "keyring", hosts: octocat, source: sourceKeyring, login: "octocat"},
		{name: "not logged in", hosts: "hosts:\n  github.com: {}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := ghLookup{
				token:  func(string) (string, string) { t.Fatal("QuickToken ran gh"); return "", "" },
				stored: func(string) (string, string) { return tt.stored, tt.from },
				config: func() (*config.Config, error) { return config.ReadFromString(tt.hosts), nil },
			}
			token, source, login := g.quickToken("github.com")
			if token != tt.token || source != tt.source || login != tt.login {
				t.Errorf("quickToken = %q, %q, %q; want %q, %q, %q", token, source, login, tt.token, tt.source, tt.login)
			}
		})
	}
}

// sentTokens is a transport that answers every request with an empty
// object, and keeps the token each was sent with.
type sentTokens struct {
	mu   sync.Mutex
	sent []string
}

func (s *sentTokens) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	s.sent = append(s.sent, strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "))
	s.mu.Unlock()
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
}

func (s *sentTokens) tokens() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sent...)
}

// laterClient returns a client whose token comes later, and what it
// sends.
func laterClient(t *testing.T) (*Client, *sentTokens) {
	t.Helper()
	sent := &sentTokens{}
	c, err := New(WithHost("github.com"), WithTokenLater(sourceKeyring), WithHTTPClient(&http.Client{Transport: sent}),
		withGH(fakeGH("", "", "hosts:\n  github.com:\n    user: octocat\n")))
	if err != nil {
		t.Fatal(err)
	}
	return c, sent
}

// A request waits for the token, and goes with it once it comes.
func TestTokenLater(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, sent := laterClient(t)
		done := make(chan error)
		go func() {
			var v any
			_, err := c.Get(t.Context(), "user", Conditional{}, &v)
			done <- err
		}()
		synctest.Wait()
		if got := sent.tokens(); len(got) != 0 {
			t.Fatalf("sent %q before the token came", got)
		}
		c.Authorize("gho_later")
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if got := sent.tokens(); len(got) != 1 || got[0] != "gho_later" {
			t.Errorf("sent %q, want the token that came", got)
		}
		c.Authorize("gho_other")
		if got := *c.token.Load(); got != "gho_later" {
			t.Errorf("a second Authorize set %q", got)
		}
		want, err := New(WithHost("github.com"), WithToken("gho_later"))
		if err != nil {
			t.Fatal(err)
		}
		if c.TokenAccount() != want.TokenAccount() || c.Access().Kind != core.TokenClassic {
			t.Errorf("TokenAccount %q, kind %v; want those of a client made with the token", c.TokenAccount(), c.Access().Kind)
		}
	})
}

// No token fails the requests that wait for it, saying how to log in, and
// a request whose context ends stops waiting.
func TestTokenLaterFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, sent := laterClient(t)
		ctx, cancel := context.WithCancel(t.Context())
		canceled := make(chan error)
		go func() {
			_, err := c.Get(ctx, "user", Conditional{}, nil)
			canceled <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-canceled; !errors.Is(err, context.Canceled) {
			t.Errorf("canceled wait = %v, want context.Canceled", err)
		}
		c.Authorize("")
		_, err := c.Get(t.Context(), "user", Conditional{}, nil)
		if !errors.Is(err, core.ErrUnauthorized) || !strings.Contains(err.Error(), "gh auth login") {
			t.Errorf("request without a token = %v, want one that says to log in", err)
		}
		if got := sent.tokens(); len(got) != 0 {
			t.Errorf("sent %q without a token", got)
		}
	})
}

// A token set while the first was still read, as by an :auth refresh, is
// the newer one, so Authorize keeps it.
func TestTokenLaterKeepsATokenSetMeanwhile(t *testing.T) {
	c, sent := laterClient(t)
	c.SetToken("gho_refreshed")
	c.Authorize("gho_first")
	var v any
	if _, err := c.Get(t.Context(), "user", Conditional{}, &v); err != nil {
		t.Fatal(err)
	}
	if got := sent.tokens(); len(got) != 1 || got[0] != "gho_refreshed" {
		t.Errorf("sent %q, want the token set meanwhile", got)
	}
}
