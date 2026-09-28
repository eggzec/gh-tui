package github

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestBodyHTML(t *testing.T) {
	c, reqs := serveFixture(t, "body_html.json")
	got, err := c.BodyHTML(t.Context(), []string{"IC_kwDOA1", "I_gone", "R_repo"})
	if err != nil {
		t.Fatalf("BodyHTML: %v", err)
	}
	req := <-reqs
	if req.Query != bodyHTMLQuery {
		t.Errorf("query = %q, want bodyHTMLQuery", req.Query)
	}
	if ids, _ := req.Variables["ids"].([]any); len(ids) != 3 || ids[0] != "IC_kwDOA1" {
		t.Errorf("ids = %v", req.Variables["ids"])
	}
	if len(got) != 1 || !strings.Contains(got["IC_kwDOA1"], "private-user-images.githubusercontent.com") {
		t.Errorf("BodyHTML = %v, want the one comment's HTML", got)
	}
}

// A hundred bodies take one request, and more take one for each hundred.
func TestBodyHTMLBatches(t *testing.T) {
	var sizes []int
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req gqlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		ids, _ := req.Variables["ids"].([]any)
		sizes = append(sizes, len(ids))
		_, _ = w.Write([]byte(`{"data":{"nodes":[]}}`))
	}))
	ids := make([]string, 250)
	for i := range ids {
		ids[i] = "IC_" + string(rune('a'+i%26))
	}
	if _, err := c.BodyHTML(t.Context(), ids); err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 3 || sizes[0] != 100 || sizes[1] != 100 || sizes[2] != 50 {
		t.Errorf("batches of %v, want 100, 100 and 50", sizes)
	}
	sizes = nil
	if _, err := c.BodyHTML(t.Context(), nil); err != nil || len(sizes) != 0 {
		t.Errorf("no IDs: %d requests, err %v; want none", len(sizes), err)
	}
}
