package github

import (
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
