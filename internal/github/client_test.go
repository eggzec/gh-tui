package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// newTestClient returns a client whose REST root and GraphQL endpoint are
// served by h.
func newTestClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(WithBaseURL(srv.URL), WithToken("test-token"), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// isolateGH points gh's config and binary at nothing, so that token lookup
// only sees the environment.
func isolateGH(t *testing.T) {
	t.Helper()
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_PATH", "/nonexistent/gh")
	t.Setenv("GH_HOST", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_ENTERPRISE_TOKEN", "")
	t.Setenv("GITHUB_ENTERPRISE_TOKEN", "")
}

func TestNewEndpoints(t *testing.T) {
	tests := []struct {
		name        string
		opts        []Option
		wantREST    string
		wantGraphQL string
		wantHost    string
	}{
		{
			name:        "github.com",
			opts:        []Option{WithHost("github.com")},
			wantREST:    "https://api.github.com/",
			wantGraphQL: "https://api.github.com/graphql",
			wantHost:    "api.github.com",
		},
		{
			name:        "enterprise server",
			opts:        []Option{WithHost("ghe.example.com")},
			wantREST:    "https://ghe.example.com/api/v3/",
			wantGraphQL: "https://ghe.example.com/api/graphql",
			wantHost:    "ghe.example.com",
		},
		{
			name:        "base URL",
			opts:        []Option{WithBaseURL("http://127.0.0.1:8080")},
			wantREST:    "http://127.0.0.1:8080/",
			wantGraphQL: "http://127.0.0.1:8080/graphql",
			wantHost:    "127.0.0.1",
		},
		{
			name:        "enterprise base URL",
			opts:        []Option{WithBaseURL("https://ghe.example.com/api/v3/")},
			wantREST:    "https://ghe.example.com/api/v3/",
			wantGraphQL: "https://ghe.example.com/api/graphql",
			wantHost:    "ghe.example.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New(append(tt.opts, WithToken("t"))...)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if got := c.restURL.String(); got != tt.wantREST {
				t.Errorf("REST root = %q, want %q", got, tt.wantREST)
			}
			if c.graphqlURL != tt.wantGraphQL {
				t.Errorf("GraphQL endpoint = %q, want %q", c.graphqlURL, tt.wantGraphQL)
			}
			if got := c.Host(); got != tt.wantHost {
				t.Errorf("Host() = %q, want %q", got, tt.wantHost)
			}
		})
	}
}

func TestWebHost(t *testing.T) {
	for base, want := range map[string]string{
		"https://api.github.com/":             "github.com",
		"https://api.acme.ghe.com/":           "acme.ghe.com",
		"https://ghe.example.com/api/v3/":     "ghe.example.com",
		"https://ghe.example.com:8443/api/v3": "ghe.example.com:8443",
		"https://api.corp.example/api/v3/":    "api.corp.example",
	} {
		c, err := New(WithBaseURL(base), WithToken("t"))
		if err != nil {
			t.Fatal(err)
		}
		if got := c.WebHost(); got != want {
			t.Errorf("WebHost() of %s = %q, want %q", base, got, want)
		}
	}
}

func TestNewTokenFromEnv(t *testing.T) {
	isolateGH(t)
	t.Setenv("GH_TOKEN", "env-token")

	c, err := New(WithHost("github.com"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.token != "env-token" {
		t.Errorf("token = %q, want env-token", c.token)
	}
	if c.http.Timeout != defaultTimeout {
		t.Errorf("timeout = %v, want %v", c.http.Timeout, defaultTimeout)
	}
}

func TestNewWithoutToken(t *testing.T) {
	isolateGH(t)

	_, err := New(WithHost("github.com"))
	if !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("New error = %v, want ErrUnauthorized", err)
	}
}

func TestRequestHeaders(t *testing.T) {
	want := map[string]string{
		"Authorization":        "Bearer test-token",
		"User-Agent":           userAgent,
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": apiVersion,
	}
	c := newTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		for k, v := range want {
			if got := r.Header.Get(k); got != v {
				t.Errorf("%s = %q, want %q", k, got, v)
			}
		}
	}))
	if _, err := c.Get(t.Context(), "user", Conditional{}, nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
}

func TestResolveRefusesOtherHosts(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("request reached the server")
	}))
	_, err := c.Get(t.Context(), "https://evil.example.com/steal", Conditional{}, nil)
	if err == nil {
		t.Fatal("Get to another host succeeded")
	}
}

func TestResolveAcceptsLeadingSlash(t *testing.T) {
	c, err := New(WithBaseURL("https://ghe.example.com/api/v3"), WithToken("t"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := c.resolve("/repos/o/r?page=2")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if want := "https://ghe.example.com/api/v3/repos/o/r?page=2"; got != want {
		t.Errorf("resolve = %q, want %q", got, want)
	}
}

func TestContextCanceled(t *testing.T) {
	arrived := make(chan struct{})
	c := newTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		<-arrived
		cancel()
	}()

	_, err := c.Get(ctx, "slow", Conditional{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Get error = %v, want context.Canceled", err)
	}
}

func TestAccount(t *testing.T) {
	account := func(host, token string) string {
		t.Helper()
		c, err := New(WithBaseURL("https://"+host+"/"), WithToken(token))
		if err != nil {
			t.Fatal(err)
		}
		return c.Account()
	}
	a := account("api.github.com", "token-a")
	if len(a) != 32 || strings.Contains(a, "token") {
		t.Errorf("Account() = %q, want 32 hex digits", a)
	}
	if again := account("api.github.com", "token-a"); again != a {
		t.Errorf("Account() = %q then %q, want it stable", a, again)
	}
	if b := account("api.github.com", "token-b"); b == a {
		t.Error("two tokens have the same account")
	}
	if ghe := account("ghe.example.com", "token-a"); ghe == a {
		t.Error("two hosts have the same account")
	}
}
