package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/config"

	"github.com/eggzec/gh-tui/internal/core"
)

// newTestClient returns a client whose REST root and GraphQL endpoint are
// served by h. It sends a request again at once, without the backoff, so
// that tests of what a failure comes to don't wait for the retries first.
func newTestClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(WithBaseURL(srv.URL), WithToken("test-token"), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return retryAtOnce(c)
}

// retryAtOnce makes c send a request again without waiting, and returns it.
func retryAtOnce(c *Client) *Client {
	c.http.Transport.(*retryTransport).wait = func(ctx context.Context, _ time.Duration) error { return ctx.Err() }
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
	if tt, ok := c.http.Transport.(*retryTransport).base.(*timeoutTransport); !ok || tt.timeout != defaultTimeout || c.http.Timeout != 0 {
		t.Errorf("timeout = %v per call and %+v per attempt, want %v per attempt", c.http.Timeout, c.http.Transport, defaultTimeout)
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

func TestNewWithoutTokenNamesHost(t *testing.T) {
	isolateGH(t)
	for host, want := range map[string]string{
		"github.com":      "run gh auth login:",
		"ghe.example.com": "run gh auth login --hostname ghe.example.com:",
	} {
		_, err := New(WithHost(host))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("New for %s without a token: error = %v, want it to say %q", host, err, want)
		}
	}
}

// withGH makes New ask lookup, rather than the user's gh, for what gh
// knows.
func withGH(lookup ghLookup) Option {
	return func(o *options) { o.gh = lookup }
}

// fakeGH is a gh logged in to github.com, whose token for it comes from
// source, and whose config is hostsYAML.
func fakeGH(token, source, hostsYAML string) ghLookup {
	return ghLookup{
		defaultHost: func() (string, string) { return "github.com", "default" },
		token:       func(string) (string, string) { return token, source },
		config:      func() (*config.Config, error) { return config.ReadFromString(hostsYAML), nil },
	}
}

func TestAccountByLogin(t *testing.T) {
	const (
		octocat = "hosts:\n  github.com:\n    user: octocat\n"
		hubot   = "hosts:\n  github.com:\n    user: hubot\n"
		shouted = "hosts:\n  github.com:\n    user: OctoCat\n"
		nobody  = "hosts:\n  github.com: {}\n"
	)
	client := func(token, source, hosts string) *Client {
		t.Helper()
		c, err := New(withGH(fakeGH(token, source, hosts)))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	for _, source := range []string{sourceKeyring, sourceHostsFile} {
		a, b := client("token-a", source, octocat), client("token-b", source, octocat)
		if a.Account() != b.Account() {
			t.Errorf("%s: a new token of the same login changed the account", source)
		}
		if a.Account() == a.TokenAccount() {
			t.Errorf("%s: the account is named by the token", source)
		}
		if a.TokenAccount() == b.TokenAccount() {
			t.Errorf("%s: two tokens have the same token account", source)
		}
		if client("token-a", source, hubot).Account() == a.Account() {
			t.Errorf("%s: two logins have the same account", source)
		}
		if client("token-a", source, shouted).Account() != a.Account() {
			t.Errorf("%s: the case of the login changed the account", source)
		}
		if c := client("token-a", source, nobody); c.Account() != c.TokenAccount() {
			t.Errorf("%s: without a login, the account isn't named by the token", source)
		}
	}
	for _, source := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"} {
		a, b := client("token-a", source, octocat), client("token-b", source, octocat)
		if a.Account() != a.TokenAccount() {
			t.Errorf("%s: a token from the environment isn't named by the token", source)
		}
		if a.Account() == b.Account() {
			t.Errorf("%s: two tokens from the environment have the same account", source)
		}
	}
	// A token given to New is named by the token, as before.
	c, err := New(WithHost("github.com"), WithToken("t"), withGH(fakeGH("", "", octocat)))
	if err != nil {
		t.Fatal(err)
	}
	if c.Account() != c.TokenAccount() {
		t.Error("a token given to New isn't named by the token")
	}
}
