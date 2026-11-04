package github

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCountIssues(t *testing.T) {
	const query = "repo:octo-org/hello is:issue is:open label:bug sort:updated-desc"
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/graphql" || !strings.Contains(string(body), `"query":"`+query+`"`) {
			t.Errorf("request %s %s\n%s", r.Method, r.URL.Path, body)
		}
		_, _ = w.Write([]byte(`{"data":{"search":{"issueCount":42}}}`))
	}))
	n, err := c.CountIssues(t.Context(), query)
	if err != nil || n != 42 {
		t.Fatalf("CountIssues = %d, %v; want 42", n, err)
	}
}

func TestCountIssuesError(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
	}))
	if _, err := c.CountIssues(t.Context(), "x"); err == nil {
		t.Fatal("CountIssues succeeded, want the error")
	}
}
