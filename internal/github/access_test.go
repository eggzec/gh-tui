package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// accessClient returns a client with token that sends its requests to h,
// and the list of what it told WithOnAccess.
func accessClient(t *testing.T, token string, h http.Handler) (c *Client, told func() []core.Access) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	var mu sync.Mutex
	var got []core.Access
	c, err := New(WithBaseURL(srv.URL), WithToken(token), WithHTTPClient(srv.Client()),
		WithOnAccess(func(a core.Access) {
			mu.Lock()
			got = append(got, a)
			mu.Unlock()
		}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return retryAtOnce(c), func() []core.Access {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(got)
	}
}

// withHeaders answers with the headers h and an empty JSON object.
func withHeaders(h map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range h {
			w.Header()[k] = []string{v}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{}")
	}
}

func TestAccessFromAnswers(t *testing.T) {
	classic := func(scopes ...string) core.Access {
		return core.Access{Kind: core.TokenClassic, Known: true, Scopes: scopes}
	}
	tests := []struct {
		name    string
		token   string
		headers map[string]string
		graphql bool
		want    core.Access
		told    bool
	}{
		{
			name:    "classic with scopes",
			token:   "gho_abc",
			headers: map[string]string{"X-OAuth-Scopes": "repo, gist, read:org", "X-Accepted-OAuth-Scopes": "repo"},
			want:    classic("gist", "read:org", "repo"),
			told:    true,
		},
		{
			name:    "graphql",
			token:   "gho_abc",
			headers: map[string]string{"X-OAuth-Scopes": "gist, read:org, repo"},
			graphql: true,
			want:    classic("gist", "read:org", "repo"),
			told:    true,
		},
		{
			name:  "classic without the header",
			token: "gho_abc",
			want:  core.Access{Kind: core.TokenClassic},
		},
		{
			name:    "classic with no scopes",
			token:   "ghp_abc",
			headers: map[string]string{"X-OAuth-Scopes": ""},
			want:    classic(),
			told:    true,
		},
		{
			name:    "the header says classic whatever the prefix",
			token:   "something",
			headers: map[string]string{"X-OAuth-Scopes": "repo"},
			want:    classic("repo"),
			told:    true,
		},
		{
			name:  "fine-grained",
			token: "github_pat_abc",
			want:  core.Access{Kind: core.TokenFineGrained},
		},
		{
			name:    "fine-grained with an empty header",
			token:   "github_pat_abc",
			headers: map[string]string{"X-OAuth-Scopes": ""},
			want:    core.Access{Kind: core.TokenFineGrained},
		},
		{
			name:    "app with an empty header",
			token:   "ghs_abc",
			headers: map[string]string{"X-OAuth-Scopes": ""},
			want:    core.Access{Kind: core.TokenApp},
		},
		{
			name:    "unknown with an empty header",
			token:   "something",
			headers: map[string]string{"X-OAuth-Scopes": ""},
			want:    classic(),
			told:    true,
		},
		{
			name:    "organizations that need SSO",
			token:   "gho_abc",
			headers: map[string]string{"X-OAuth-Scopes": "repo", "X-GitHub-SSO": "partial-results; organizations=21955855, 20582480"},
			want:    core.Access{Kind: core.TokenClassic, Known: true, Scopes: []string{"repo"}, SSO: []string{"20582480", "21955855"}},
			told:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, told := accessClient(t, tt.token, withHeaders(tt.headers))
			for range 2 {
				var err error
				if tt.graphql {
					err = c.Query(t.Context(), "query { viewer { login } }", nil, nil)
				} else {
					_, err = c.Get(t.Context(), "user", Conditional{}, nil)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := c.Access(); !got.Equal(tt.want) {
				t.Errorf("Access() = %+v, want %+v", got, tt.want)
			}
			// The same answer twice tells once.
			var want []core.Access
			if tt.told {
				want = []core.Access{tt.want}
			}
			if got := told(); !slices.EqualFunc(got, want, core.Access.Equal) {
				t.Errorf("told %+v, want %+v", got, want)
			}
		})
	}
}

// An answer without the header, or from another host, changes nothing: a
// classic token's scopes stay what GitHub said last.
func TestAccessKeptWithoutHeader(t *testing.T) {
	scopes := "repo"
	var mu sync.Mutex
	c, told := accessClient(t, "gho_abc", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		if scopes != "" {
			w.Header().Set("X-OAuth-Scopes", scopes)
		}
		mu.Unlock()
		_, _ = io.WriteString(w, "{}")
	}))
	get := func() {
		t.Helper()
		if _, err := c.Get(t.Context(), "user", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}
	}
	get()
	mu.Lock()
	scopes = ""
	mu.Unlock()
	get()
	want := core.Access{Kind: core.TokenClassic, Known: true, Scopes: []string{"repo"}}
	if got := c.Access(); !got.Equal(want) {
		t.Errorf("Access() = %+v, want %+v", got, want)
	}
	mu.Lock()
	scopes = "repo, workflow"
	mu.Unlock()
	get()
	want.Scopes = []string{"repo", "workflow"}
	if got := told(); len(got) != 2 || !got[1].Equal(want) {
		t.Errorf("told %+v, want the scopes and then %+v", got, want)
	}
}

// An answer to a request sent with the token before SetToken says
// nothing of the new one, however late it comes.
func TestAccessIgnoresOldToken(t *testing.T) {
	arrived, release := make(chan struct{}), make(chan struct{})
	c, _ := accessClient(t, "gho_old", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer gho_old" {
			close(arrived)
			<-release
			w.Header().Set("X-OAuth-Scopes", "gist")
		} else {
			w.Header().Set("X-OAuth-Scopes", "repo, workflow")
		}
		_, _ = io.WriteString(w, "{}")
	}))
	done := make(chan error)
	go func() {
		_, err := c.Get(t.Context(), "user", Conditional{}, nil)
		done <- err
	}()
	<-arrived
	c.SetToken("gho_new")
	if err := c.ProbeAccess(t.Context()); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	want := core.Access{Kind: core.TokenClassic, Known: true, Scopes: []string{"repo", "workflow"}}
	if got := c.Access(); !got.Equal(want) {
		t.Errorf("Access() = %+v, want %+v", got, want)
	}
}

// onAccess may replace the token itself, and is told of that once it
// returns.
func TestAccessSetTokenWhenTold(t *testing.T) {
	var a *tokenAccess
	var told []core.Access
	a = newTokenAccess("api.github.com", "gho_abc", func(acc core.Access) {
		told = append(told, acc)
		if len(told) == 1 {
			a.reset("github_pat_new")
		}
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/user", http.NoBody)
	req.Header.Set("Authorization", "Bearer gho_abc")
	a.observe(req, http.Header{"X-Oauth-Scopes": {"repo"}})
	want := []core.Access{
		{Kind: core.TokenClassic, Known: true, Scopes: []string{"repo"}},
		{Kind: core.TokenFineGrained},
	}
	if !slices.EqualFunc(told, want, core.Access.Equal) {
		t.Errorf("told %+v, want %+v", told, want)
	}
}

// The storage a job's log redirects to isn't GitHub's API, whatever its
// headers say, even on the API's own host.
func TestAccessIgnoresDownloads(t *testing.T) {
	blob := httptest.NewServer(withHeaders(map[string]string{"X-OAuth-Scopes": "gist"}))
	t.Cleanup(blob.Close)
	for name, elsewhere := range map[string]bool{"another host": true, "the API host": false} {
		t.Run(name, func(t *testing.T) {
			var c *Client
			c, _ = accessClient(t, "gho_abc", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/storage/log":
					w.Header().Set("X-OAuth-Scopes", "gist")
					_, _ = io.WriteString(w, "log")
				case elsewhere:
					w.Header().Set("X-OAuth-Scopes", "repo")
					http.Redirect(w, r, blob.URL+"/storage/log", http.StatusFound)
				default:
					w.Header().Set("X-OAuth-Scopes", "repo")
					http.Redirect(w, r, "http://"+r.Host+"/storage/log", http.StatusFound)
				}
			}))
			if _, _, err := c.JobLog(t.Context(), core.RepoRef{Owner: "eggzec", Name: "gh-tui"}, 7, 1<<20); err != nil {
				t.Fatal(err)
			}
			want := core.Access{Kind: core.TokenClassic, Known: true, Scopes: []string{"repo"}}
			if got := c.Access(); !got.Equal(want) {
				t.Errorf("Access() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestSetToken(t *testing.T) {
	var mu sync.Mutex
	var auth []string
	c, told := accessClient(t, "gho_old", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth = append(auth, r.Header.Get("Authorization"))
		mu.Unlock()
		if r.Header.Get("Authorization") == "Bearer gho_old" {
			w.Header().Set("X-OAuth-Scopes", "repo")
		} else {
			w.Header().Set("X-OAuth-Scopes", "repo, workflow")
		}
		_, _ = io.WriteString(w, "{}")
	}))
	account := c.Account()
	if err := c.ProbeAccess(t.Context()); err != nil {
		t.Fatal(err)
	}
	c.SetToken("github_pat_new")
	if got, want := c.Access(), (core.Access{Kind: core.TokenFineGrained}); !got.Equal(want) {
		t.Errorf("Access() after SetToken = %+v, want %+v", got, want)
	}
	if err := c.ProbeAccess(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, want := auth, []string{"Bearer gho_old", "Bearer github_pat_new"}; !slices.Equal(got, want) {
		t.Errorf("sent %q, want %q", got, want)
	}
	if c.Account() != account {
		t.Error("Account changed with the token")
	}
	want := []core.Access{
		{Kind: core.TokenClassic, Known: true, Scopes: []string{"repo"}},
		{Kind: core.TokenFineGrained},
		{Kind: core.TokenClassic, Known: true, Scopes: []string{"repo", "workflow"}},
	}
	if got := told(); !slices.EqualFunc(got, want, core.Access.Equal) {
		t.Errorf("told %+v, want %+v", got, want)
	}
}

func TestProbeAccess(t *testing.T) {
	tests := []struct {
		name   string
		status int
		fails  bool
	}{
		{"ok", http.StatusOK, false},
		// An Enterprise Server that doesn't limit the rate.
		{"not found", http.StatusNotFound, false},
		{"bad credentials", http.StatusUnauthorized, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := 0
			c, _ := accessClient(t, "ghp_abc", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				if r.URL.Path != "/rate_limit" {
					t.Errorf("probed %s, want /rate_limit", r.URL.Path)
				}
				w.Header().Set("X-OAuth-Scopes", "repo")
				w.WriteHeader(tt.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"resources": map[string]any{}})
			}))
			err := c.ProbeAccess(t.Context())
			if (err != nil) != tt.fails {
				t.Errorf("ProbeAccess = %v, want an error: %v", err, tt.fails)
			}
			if hits != 1 {
				t.Errorf("probed %d times, want once", hits)
			}
			want := core.Access{Kind: core.TokenClassic, Known: true, Scopes: []string{"repo"}}
			if got := c.Access(); !got.Equal(want) {
				t.Errorf("Access() = %+v, want %+v", got, want)
			}
		})
	}
}

// TestAccessErrors checks what core.Explain makes of GitHub's refusals of
// what the token may do.
func TestAccessErrors(t *testing.T) {
	const workflowMsg = "refusing to allow an OAuth App to create or update workflow `.github/workflows/ci.yml` without `workflow` scope"
	tests := []struct {
		name    string
		h       http.Handler
		graphql bool
		kind    core.ProblemKind
		reason  string
		grant   string
		sso     bool
		ssoURL  string // with %s for the server's host
	}{
		{
			name:   "classic 403 without the scope",
			h:      reply(403, map[string]string{"X-OAuth-Scopes": "gist", "X-Accepted-OAuth-Scopes": "notifications, repo"}, "Not allowed"),
			kind:   core.Auth,
			reason: "Not allowed",
			grant:  "notifications",
		},
		{
			name: "fine-grained 403",
			h: reply(403, map[string]string{"X-Accepted-GitHub-Permissions": "pull_requests=write,contents=read"},
				"Resource not accessible by personal access token"),
			kind:   core.Forbidden,
			reason: "Resource not accessible by personal access token (needs Pull requests: write, Contents: read)",
		},
		{
			name:   "permissions that would each do",
			h:      reply(403, map[string]string{"X-Accepted-GitHub-Permissions": "issues=write; pull_requests=write"}, "Resource not accessible by integration"),
			kind:   core.Forbidden,
			reason: "Resource not accessible by integration (needs Issues: write or Pull requests: write)",
		},
		{
			name:   "permissions it can't read",
			h:      reply(403, map[string]string{"X-Accepted-GitHub-Permissions": "issues=write, $(rm -rf ~)=admin"}, "Resource not accessible by integration"),
			kind:   core.Forbidden,
			reason: "Resource not accessible by integration",
		},
		{
			name: "sso",
			h: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reply(403, map[string]string{"X-GitHub-SSO": "required; url=http://" + r.Host + "/orgs/eggzec/sso?authorization_request=x"}, ssoMessage)(w, r)
			}),
			kind:   core.Forbidden,
			reason: ssoMessage,
			sso:    true,
			ssoURL: "http://%s/orgs/eggzec/sso?authorization_request=x",
		},
		{
			name: "sso with a user and the host in capitals",
			h: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reply(403, map[string]string{"X-GitHub-SSO": "required; url=http://me:pw@" + strings.ToUpper(r.Host) + "/orgs/eggzec/sso"}, ssoMessage)(w, r)
			}),
			kind:   core.Forbidden,
			reason: ssoMessage,
			sso:    true,
			ssoURL: "http://%s/orgs/eggzec/sso",
		},
		{
			name:   "sso elsewhere",
			h:      reply(403, map[string]string{"X-GitHub-SSO": "required; url=https://evil.example/sso"}, ssoMessage),
			kind:   core.Forbidden,
			reason: ssoMessage,
			sso:    true,
		},
		{
			name:   "sso without a url",
			h:      reply(403, map[string]string{"X-GitHub-SSO": "required"}, "Forbidden"),
			kind:   core.Forbidden,
			reason: "Forbidden",
			sso:    true,
		},
		{
			name:   "permissions too long to read",
			h:      reply(403, map[string]string{"X-Accepted-GitHub-Permissions": strings.Repeat("issues=write,", 100) + "issues=write"}, "Resource not accessible by integration"),
			kind:   core.Forbidden,
			reason: "Resource not accessible by integration",
		},
		{
			name:    "graphql insufficient scopes",
			h:       graphqlReply("INSUFFICIENT_SCOPES", "The 'teams' field requires one of the following scopes: ['read:org', 'admin:org'], but your token has only been granted the: ['repo'] scopes.", map[string]string{"X-OAuth-Scopes": "repo"}),
			graphql: true,
			kind:    core.Auth,
			reason:  "The 'teams' field requires one of the following scopes: ['read:org', 'admin:org'], but your token has only been granted the: ['repo'] scopes.",
			grant:   "read:org",
		},
		{
			name:    "graphql workflow",
			h:       graphqlReply("UNPROCESSABLE", workflowMsg, nil),
			graphql: true,
			kind:    core.Auth,
			reason:  workflowMsg,
			grant:   "workflow",
		},
		{
			name:    "graphql workflow of an app",
			h:       graphqlReply("", "refusing to allow a GitHub App to create or update workflow `.github/workflows/ci.yml` without `workflows` permission", nil),
			graphql: true,
			kind:    core.Forbidden,
			reason:  "refusing to allow a GitHub App to create or update workflow `.github/workflows/ci.yml` without `workflows` permission",
		},
		{
			name:   "rest workflow",
			h:      reply(422, nil, workflowMsg),
			kind:   core.Auth,
			reason: workflowMsg,
			grant:  "workflow",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := accessClient(t, "gho_abc", tt.h)
			var err error
			if tt.graphql {
				err = c.Query(t.Context(), "mutation { merge }", nil, nil)
			} else {
				_, err = c.Get(t.Context(), "repos/eggzec/gh-tui", Conditional{}, nil)
			}
			p := core.Explain("merge #5", fmt.Errorf("merge: %w", err))
			ssoURL := tt.ssoURL
			if ssoURL != "" {
				ssoURL = fmt.Sprintf(ssoURL, c.WebHost())
			}
			if p.Kind != tt.kind || p.Reason != tt.reason || p.Grant != tt.grant || p.SSO != tt.sso || p.SSOURL != ssoURL {
				t.Errorf("Explain(%v) = {%v %q %q %v %q}, want {%v %q %q %v %q}", err,
					p.Kind, p.Reason, p.Grant, p.SSO, p.SSOURL, tt.kind, tt.reason, tt.grant, tt.sso, ssoURL)
			}
		})
	}
}

func TestTokenKind(t *testing.T) {
	for token, want := range map[string]core.TokenKind{
		"gho_16C7e42F292c6912E7710c838347Ae178B4a": core.TokenClassic,
		"ghp_16C7e42F292c6912E7710c838347Ae178B4a": core.TokenClassic,
		"0123456789abcdef0123456789abcdef01234567": core.TokenClassic,
		"github_pat_11ABCDEFG0123456789_abcdefg":   core.TokenFineGrained,
		"ghu_16C7e42F292c6912E7710c838347Ae178B4a": core.TokenApp,
		"ghs_16C7e42F292c6912E7710c838347Ae178B4a": core.TokenApp,
		"0123456789abcdef0123456789abcdef0123456":  core.TokenUnknown,
		"0123456789abcdef0123456789abcdef0123456z": core.TokenUnknown,
		"test-token": core.TokenUnknown,
		"":           core.TokenUnknown,
	} {
		if got := tokenKind(token); got != want {
			t.Errorf("tokenKind(%q) = %v, want %v", token, got, want)
		}
	}
}

// Answers that arrive at once tell the latest access, and never an older
// one after a newer.
func TestAccessToldInOrder(t *testing.T) {
	var mu sync.Mutex
	var told []core.Access
	a := newTokenAccess("api.github.com", "gho_abc", func(acc core.Access) {
		mu.Lock()
		told = append(told, acc)
		mu.Unlock()
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/user", http.NoBody)
	req.Header.Set("Authorization", "Bearer gho_abc")
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			a.observe(req, http.Header{"X-Oauth-Scopes": {fmt.Sprint("repo, s", i%2)}})
		})
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(told) == 0 || !told[len(told)-1].Equal(a.get()) {
		t.Errorf("told %+v last, want %+v", told, a.get())
	}
}

func TestSSOLink(t *testing.T) {
	c, err := New(WithBaseURL("https://api.github.com/"), WithToken("t"))
	if err != nil {
		t.Fatal(err)
	}
	for u, want := range map[string]string{
		"https://github.com/orgs/x/sso?authorization_request=a": "https://github.com/orgs/x/sso?authorization_request=a",
		"https://GitHub.com/orgs/x/sso":                         "https://GitHub.com/orgs/x/sso",
		"https://github.com:443/orgs/x/sso":                     "https://github.com:443/orgs/x/sso",
		"https://me:pw@github.com/orgs/x/sso":                   "https://github.com/orgs/x/sso",
		"https://github.com:8443/orgs/x/sso":                    "",
		"http://github.com/orgs/x/sso":                          "",
		"javascript:alert(1)":                                   "",
		"https://github.com.evil.example/sso":                   "",
	} {
		if got := c.ssoLink(u); got != want {
			t.Errorf("ssoLink(%q) = %q, want %q", u, got, want)
		}
	}
}
