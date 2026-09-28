package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
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
