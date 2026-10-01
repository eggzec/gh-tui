package github

import (
	"net/http"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestRepoUsers(t *testing.T) {
	c, reqs := pullServer(t, "users_search.json")
	users, err := c.RepoUsers(t.Context(), pullsRepo, " meow ", 10)
	if err != nil {
		t.Fatalf("RepoUsers: %v", err)
	}
	checkPullQuery(t, reqs(), "assignableUsers(query: $query", map[string]any{
		"owner": "eggzec", "name": "gh-tui", "query": "meow", "first": float64(10), "search": "meow in:login", "typed": true,
	})
	logins := make([]string, len(users))
	for i, u := range users {
		logins[i] = u.Login
	}
	// The assignable one comes first and only once, and the organization
	// is left out.
	if want := []string{"meowgorithm", "Meow", "meowtech0x", "meowtec"}; !slices.Equal(logins, want) {
		t.Errorf("logins = %v, want %v", logins, want)
	}
	if users[0] != (core.User{Login: "meowgorithm", Name: "Christian Rocha"}) {
		t.Errorf("first = %+v, want the name too", users[0])
	}
}

func TestRepoUsersUntyped(t *testing.T) {
	c, reqs := pullServer(t, "users_search.json")
	if _, err := c.RepoUsers(t.Context(), pullsRepo, "", 20); err != nil {
		t.Fatalf("RepoUsers: %v", err)
	}
	checkPullQuery(t, reqs(), "RepoUsers", map[string]any{
		"owner": "eggzec", "name": "gh-tui", "first": float64(20), "search": "", "typed": false,
	})
}

func TestViewerLogin(t *testing.T) {
	c, reqs := pullServer(t, "viewer_login.json")
	login, err := c.ViewerLogin(t.Context())
	if err != nil || login != "octocat" {
		t.Fatalf("ViewerLogin = %q, %v; want octocat", login, err)
	}
	if n := len(reqs()); n != 1 {
		t.Errorf("sent %d requests, want 1", n)
	}
}

// UserLogin reads the login from GET /user, and an unchanged account
// answers 304 to its ETag.
func TestUserLogin(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("path = %s, want /user", r.URL.Path)
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(`{"login":"octocat","id":1}`))
	}))
	login, resp, err := c.UserLogin(t.Context(), Conditional{})
	if err != nil || login != "octocat" || resp.ETag != `"v1"` {
		t.Fatalf("UserLogin = %q, %+v, %v; want octocat with its ETag", login, resp, err)
	}
	login, resp, err = c.UserLogin(t.Context(), Conditional{ETag: resp.ETag})
	if err != nil || login != "" || !resp.NotModified {
		t.Errorf("UserLogin again = %q, %+v, %v; want not modified", login, resp, err)
	}
}

// An answer without a login, or with one GitHub couldn't have, such as
// one with escape sequences, is an error, not a login.
func TestUserLoginInvalid(t *testing.T) {
	for _, body := range []string{`{}`, `{"login":"octo\u001b[2Jcat"}`, `{"login":"octo cat"}`} {
		c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		if login, _, err := c.UserLogin(t.Context(), Conditional{}); err == nil {
			t.Errorf("UserLogin(%s) = %q, want an error", body, login)
		}
	}
}
