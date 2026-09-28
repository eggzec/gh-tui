package github

import (
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestRequestsAskForGzip checks that the client sends its requests as
// production builds it, through the default transport, which asks for
// gzip and decompresses the answer on its own, unless a request sets
// Accept-Encoding itself or the transport disables compression.
func TestRequestsAskForGzip(t *testing.T) {
	var (
		mu    sync.Mutex
		asked = map[string]string{}
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked[r.URL.Path] = r.Header.Get("Accept-Encoding")
		mu.Unlock()
		body := `{"full_name":"cli/cli"}`
		if r.URL.Path == "/graphql" {
			body = `{"data":{"viewer":{"login":"octocat"}}}`
		}
		w.Header().Set("Content-Type", "application/json")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			_, _ = w.Write([]byte(body))
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = zw.Write([]byte(body))
		_ = zw.Close()
	}))
	t.Cleanup(srv.Close)
	// No WithHTTPClient: the client's transports end in the default one,
	// as they do in production.
	c, err := New(WithBaseURL(srv.URL), WithToken("test-token"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(c.Close)

	var repo struct {
		FullName string `json:"full_name"`
	}
	if _, err := c.rest(t.Context(), http.MethodGet, "repos/cli/cli", Conditional{}, nil, &repo); err != nil {
		t.Fatalf("REST: %v", err)
	}
	var data struct {
		Viewer struct{ Login string } `json:"viewer"`
	}
	if err := c.query(t.Context(), "query { viewer { login } }", nil, &data); err != nil {
		t.Fatalf("GraphQL: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, path := range []string{"/repos/cli/cli", "/graphql"} {
		if got := asked[path]; !strings.Contains(got, "gzip") {
			t.Errorf("%s: requests no longer ask for gzip (Accept-Encoding %q): a transport change disabled compression", path, got)
		}
	}
	if repo.FullName != "cli/cli" || data.Viewer.Login != "octocat" {
		t.Errorf("got %q and %q, want cli/cli and octocat: the gzip answers weren't decompressed", repo.FullName, data.Viewer.Login)
	}
}
