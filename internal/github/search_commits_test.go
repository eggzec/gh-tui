package github

import (
	"errors"
	"net/http"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

const commitsBody = `{"total_count": 2, "incomplete_results": false, "items": [
	{"sha": "aaa111", "commit": {"message": "fix: a bug\n\nlong body", "author": {"name": "Ann", "date": "2026-01-02T03:04:05Z"}},
	 "author": {"login": "ann"}, "repository": {"full_name": "o/r"}},
	{"sha": "bbb222", "commit": {"message": "wip", "author": {"name": "Bob B", "date": "2026-01-03T00:00:00Z"}},
	 "author": null, "repository": {"full_name": "o/s"}}]}`

func serveCommits(t *testing.T, query, body string) *Client {
	t.Helper()
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/search/commits", map[string]string{"q": query, "per_page": "20"})
		w.Header().Set("Link", `<http://`+r.Host+`/search/commits?q=x&page=2>; rel="next"`)
		w.Header().Set("X-RateLimit-Limit", "30")
		w.Header().Set("X-RateLimit-Remaining", "29")
		w.Header().Set("X-RateLimit-Reset", "1790252795")
		w.Header().Set("X-RateLimit-Resource", "search")
		_, _ = w.Write([]byte(body))
	}))
}

func TestSearchCommits(t *testing.T) {
	c := serveCommits(t, "fix repo:o/r", commitsBody)
	page, err := c.SearchCommits(t.Context(), "fix repo:o/r", "", 20)
	if err != nil {
		t.Fatalf("SearchCommits: %v", err)
	}
	if page.Total != 2 || page.Incomplete || len(page.Items) != 2 || page.Next == "" {
		t.Fatalf("page = total %d, incomplete %t, %d items, next %q", page.Total, page.Incomplete, len(page.Items), page.Next)
	}
	a, b := page.Items[0], page.Items[1]
	if a.Repo != (core.RepoRef{Owner: "o", Name: "r"}) || a.SHA != "aaa111" || a.Subject != "fix: a bug" || a.Author != "ann" || a.Date.Year() != 2026 {
		t.Errorf("item 0 = %+v", a)
	}
	if b.Author != "Bob B" || b.Repo.Name != "s" {
		t.Errorf("item 1 = %+v, want the commit author's name for a null account", b)
	}
}

func TestSearchCommitsBudget(t *testing.T) {
	c := serveCommits(t, "fix", commitsBody)
	if _, err := c.SearchCommits(t.Context(), "fix", "", 20); err != nil {
		t.Fatal(err)
	}
	if got := c.RateLimit(resourceSearch); got.Resource != "search" || got.Limit != 30 || got.Remaining != 29 {
		t.Errorf("RateLimit = %+v, want the search quota", got)
	}
}

func TestSearchCommitsError(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message": "Validation Failed"}`))
	}))
	_, err := c.SearchCommits(t.Context(), "x", "", 0)
	var ghErr *Error
	if !errors.As(err, &ghErr) || ghErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("error = %v, want a 422", err)
	}
}
