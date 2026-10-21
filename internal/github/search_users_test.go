package github

import (
	"errors"
	"net/http"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

const usersBody = `{"total_count": 2, "incomplete_results": false, "items": [
	{"login": "charmbracelet", "id": 1, "type": "Organization", "followers_url": "x"},
	{"login": "muesli", "id": 2, "type": "User"}]}`

func serveUsers(t *testing.T, query, body string) *Client {
	t.Helper()
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/search/users", map[string]string{"q": query, "per_page": "20"})
		w.Header().Set("Link", `<http://`+r.Host+`/search/users?q=x&page=2>; rel="next"`)
		w.Header().Set("X-RateLimit-Limit", "30")
		w.Header().Set("X-RateLimit-Remaining", "29")
		w.Header().Set("X-RateLimit-Reset", "1790252795")
		w.Header().Set("X-RateLimit-Resource", "search")
		_, _ = w.Write([]byte(body))
	}))
}

func TestSearchUsers(t *testing.T) {
	c := serveUsers(t, "tui type:user", usersBody)
	page, err := c.SearchUsers(t.Context(), "tui type:user", "", 20)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if page.Total != 2 || page.Incomplete || len(page.Items) != 2 || page.Next == "" {
		t.Fatalf("page = total %d, incomplete %t, %d items, next %q", page.Total, page.Incomplete, len(page.Items), page.Next)
	}
	if got := page.Items[1]; got != (core.SearchUser{Login: "muesli", Kind: core.OwnerUser}) {
		t.Errorf("item 1 = %+v, want the user muesli", got)
	}
}

func TestSearchOrgs(t *testing.T) {
	c := serveUsers(t, "charm type:org", usersBody)
	page, err := c.SearchUsers(t.Context(), "charm type:org", "", 20)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if got := page.Items[0]; got != (core.SearchUser{Login: "charmbracelet", Kind: core.OwnerOrg}) {
		t.Errorf("item 0 = %+v, want the organization charmbracelet", got)
	}
}

func TestSearchUsersCursor(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/users" || r.URL.Query().Get("q") != "x" || r.URL.Query().Get("page") != "2" {
			t.Errorf("request = %s, want the page 2 cursor followed", r.URL)
		}
		_, _ = w.Write([]byte(usersBody))
	}))
	// The query and page size are ignored once there is a cursor.
	page, err := c.SearchUsers(t.Context(), "ignored", "search/users?q=x&page=2", 50)
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("SearchUsers = %+v, %v", page, err)
	}
}

func TestSearchUsersBudget(t *testing.T) {
	c := serveUsers(t, "tui", usersBody)
	if _, err := c.SearchUsers(t.Context(), "tui", "", 20); err != nil {
		t.Fatal(err)
	}
	if got := c.RateLimit(resourceSearch); got.Resource != "search" || got.Limit != 30 || got.Remaining != 29 {
		t.Errorf("RateLimit = %+v, want the search quota", got)
	}
}

func TestSearchUsersError(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message": "Validation Failed"}`))
	}))
	_, err := c.SearchUsers(t.Context(), "x", "", 0)
	var ghErr *Error
	if !errors.As(err, &ghErr) || ghErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("error = %v, want a 422", err)
	}
}
