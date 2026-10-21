package github

import (
	"errors"
	"net/http"
	"testing"
)

const topicsBody = `{"total_count": 2, "incomplete_results": false, "items": [
	{"name": "machine-learning", "display_name": "Machine learning", "short_description": "Teaching computers\nto learn", "featured": true, "score": 1.5},
	{"name": "go", "display_name": null, "short_description": null, "featured": false, "score": 1.0}]}`

func serveTopics(t *testing.T, query, body string) *Client {
	t.Helper()
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/search/topics", map[string]string{"q": query, "per_page": "20"})
		w.Header().Set("Link", `<http://`+r.Host+`/search/topics?q=x&page=2>; rel="next"`)
		w.Header().Set("X-RateLimit-Limit", "30")
		w.Header().Set("X-RateLimit-Remaining", "29")
		w.Header().Set("X-RateLimit-Reset", "1790252795")
		w.Header().Set("X-RateLimit-Resource", "search")
		_, _ = w.Write([]byte(body))
	}))
}

func TestSearchTopics(t *testing.T) {
	c := serveTopics(t, "learning is:featured", topicsBody)
	page, err := c.SearchTopics(t.Context(), "learning is:featured", "", 20)
	if err != nil {
		t.Fatalf("SearchTopics: %v", err)
	}
	if page.Total != 2 || page.Incomplete || len(page.Items) != 2 || page.Next == "" {
		t.Fatalf("page = total %d, incomplete %t, %d items, next %q", page.Total, page.Incomplete, len(page.Items), page.Next)
	}
	a, b := page.Items[0], page.Items[1]
	if a.Name != "machine-learning" || a.ShortDescription != "Teaching computers to learn" {
		t.Errorf("item 0 = %+v", a)
	}
	if b.Name != "go" || b.ShortDescription != "" {
		t.Errorf("item 1 = %+v, want no description for a null one", b)
	}
}

func TestSearchTopicsBudget(t *testing.T) {
	c := serveTopics(t, "go", topicsBody)
	if _, err := c.SearchTopics(t.Context(), "go", "", 20); err != nil {
		t.Fatal(err)
	}
	if got := c.RateLimit(resourceSearch); got.Resource != "search" || got.Limit != 30 || got.Remaining != 29 {
		t.Errorf("RateLimit = %+v, want the search quota", got)
	}
}

func TestSearchTopicsError(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message": "Validation Failed"}`))
	}))
	_, err := c.SearchTopics(t.Context(), "x", "", 0)
	var ghErr *Error
	if !errors.As(err, &ghErr) || ghErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("error = %v, want a 422", err)
	}
}
