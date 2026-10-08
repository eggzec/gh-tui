package github

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

const filesFixture = `[
 {"sha":"a1","filename":"main.go","status":"modified","additions":3,"deletions":1,"changes":4,"patch":"@@ -1 +1,3 @@\n-a\n+b\n+c\n+d"},
 {"sha":"a2","filename":"new.go","previous_filename":"old.go","status":"renamed","additions":0,"deletions":0,"changes":0},
 {"sha":"a3","filename":"logo.png","status":"added","additions":0,"deletions":0,"changes":0},
 {"sha":"a4","filename":"big.json","status":"modified","additions":9000,"deletions":10,"changes":9010}
]`

func TestListPullRequestFiles(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/octo/hello/pulls/7/files" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("If-None-Match"); got != `W/"e0"` {
			t.Errorf("If-None-Match = %q, want the cached ETag", got)
		}
		q := r.URL.Query()
		w.Header().Set("ETag", `W/"e1"`)
		if q.Get("page") == "" {
			if q.Get("per_page") != "100" {
				t.Errorf("per_page = %q, want 100", q.Get("per_page"))
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/octo/hello/pulls/7/files?per_page=100&page=2>; rel="next"`, srv.URL))
			_, _ = io.WriteString(w, filesFixture)
			return
		}
		_, _ = io.WriteString(w, `[{"sha":"b1","filename":"z.go","status":"added","additions":1,"deletions":0,"changes":1,"patch":"@@ -0,0 +1 @@\n+z"}]`)
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithBaseURL(srv.URL), WithToken("t"), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}

	repo := core.RepoRef{Owner: "octo", Name: "hello"}
	p, res, err := c.ListPullRequestFiles(t.Context(), repo, 7, "", Conditional{ETag: `W/"e0"`})
	if err != nil {
		t.Fatalf("ListPullRequestFiles: %v", err)
	}
	if res.ETag != `W/"e1"` || !strings.Contains(p.Next, "page=2") || p.Truncated {
		t.Errorf("ETag = %q, Next = %q, Truncated = %v; want the ETag and a next page", res.ETag, p.Next, p.Truncated)
	}
	want := []core.CommitFile{
		{Path: "main.go", Status: core.FileModified, SHA: "a1", Additions: 3, Deletions: 1, Patch: "@@ -1 +1,3 @@\n-a\n+b\n+c\n+d"},
		{Path: "new.go", PreviousPath: "old.go", Status: core.FileRenamed, SHA: "a2"},
		{Path: "logo.png", Status: core.FileAdded, SHA: "a3"},
		{Path: "big.json", Status: core.FileModified, SHA: "a4", Additions: 9000, Deletions: 10, PatchTruncated: true},
	}
	if len(p.Items) != len(want) {
		t.Fatalf("got %d files, want %d", len(p.Items), len(want))
	}
	for i := range want {
		if p.Items[i] != want[i] {
			t.Errorf("file %d = %+v, want %+v", i, p.Items[i], want[i])
		}
	}

	next, _, err := c.ListPullRequestFiles(t.Context(), repo, 7, p.Next, Conditional{ETag: `W/"e0"`})
	if err != nil || len(next.Items) != 1 || next.Items[0].Path != "z.go" || next.Next != "" || next.Truncated {
		t.Errorf("second page = %+v, %v; want z.go, last", next, err)
	}
}

func TestListPullRequestFilesNotModified(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `W/"e1"` {
			t.Errorf("If-None-Match = %q", r.Header.Get("If-None-Match"))
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	p, res, err := c.ListPullRequestFiles(t.Context(), core.RepoRef{Owner: "o", Name: "r"}, 1, "", Conditional{ETag: `W/"e1"`})
	if err != nil || !res.NotModified || len(p.Items) != 0 {
		t.Errorf("= %+v, %+v, %v; want NotModified and no items", p, res, err)
	}
}

func TestListPullRequestFilesTruncated(t *testing.T) {
	// The first page says where the last is, and GitHub serves 30 pages
	// of 100 at most.
	tests := []struct {
		last string
		want bool
	}{
		{"", false},
		{"page=3", false},
		{"page=29", false},
		{"page=30", true},
		{"page=31", true},
	}
	for _, tt := range tests {
		c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tt.last != "" {
				w.Header().Set("Link", `<`+"http://"+r.Host+`/repos/o/r/pulls/1/files?per_page=100&`+tt.last+`>; rel="last"`)
			}
			_, _ = io.WriteString(w, `[]`)
		}))
		repo := core.RepoRef{Owner: "o", Name: "r"}
		first, _, err := c.ListPullRequestFiles(t.Context(), repo, 1, "", Conditional{})
		if err != nil || first.Truncated != tt.want {
			t.Errorf("last %q: Truncated = %v, %v; want %v", tt.last, first.Truncated, err, tt.want)
		}
		// A later page doesn't say, so the first page's word stands.
		later, _, err := c.ListPullRequestFiles(t.Context(), repo, 1, "repos/o/r/pulls/1/files?per_page=100&page=2", Conditional{})
		if err != nil || later.Truncated {
			t.Errorf("last %q: later page Truncated = %v, %v; want false", tt.last, later.Truncated, err)
		}
	}
}

func TestListPullRequestFilesNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	_, _, err := c.ListPullRequestFiles(t.Context(), core.RepoRef{Owner: "o", Name: "r"}, 1, "", Conditional{})
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("err = %v, want not found", err)
	}
}

func TestListPullRequestFilesEnterprise(t *testing.T) {
	// A GitHub Enterprise Server API lives under /api/v3.
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithBaseURL(srv.URL+"/api/v3"), WithToken("t"), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.ListPullRequestFiles(t.Context(), core.RepoRef{Owner: "o", Name: "r"}, 3, "", Conditional{}); err != nil {
		t.Fatal(err)
	}
	if got != "/api/v3/repos/o/r/pulls/3/files" {
		t.Errorf("path = %s, want it under the host's API base", got)
	}
}
