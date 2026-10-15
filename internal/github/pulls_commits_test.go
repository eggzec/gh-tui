package github

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// commitsJSON is n commits of a pull request, numbered from first.
func commitsJSON(first, n int) string {
	items := make([]string, n)
	for i := range items {
		k := first + i
		items[i] = fmt.Sprintf(`{"sha":"%040d","commit":{
			"author":{"name":"A","email":"a@example.com","date":"2026-01-02T03:04:05Z"},
			"committer":{"name":"C","email":"c@example.com","date":"2026-01-02T03:04:06Z"},
			"message":"commit %d\n\nbody"},
			"author":{"login":"octocat"},"parents":[]}`, k, k)
	}
	return "[" + strings.Join(items, ",") + "]"
}

// pullCommitsServer serves total commits of #7 in pages of 100, with the
// links GitHub gives, and records each request.
type pullCommitsServer struct {
	mu    sync.Mutex
	total int
	conds []string
	pages []string
	srv   *httptest.Server
}

func newPullCommitsServer(t *testing.T, total int) (*pullCommitsServer, *Client) {
	t.Helper()
	s := &pullCommitsServer{total: total}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.URL.Path != "/repos/octo/hello/pulls/7/commits" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q, want 100", got)
		}
		page := 1
		if p := r.URL.Query().Get("page"); p != "" {
			page, _ = strconv.Atoi(p)
		}
		s.conds = append(s.conds, r.Header.Get("If-None-Match"))
		s.pages = append(s.pages, strconv.Itoa(page))
		if r.Header.Get("If-None-Match") == `W/"e1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", fmt.Sprintf(`W/"e%d"`, page))
		first := (page - 1) * 100
		n := min(100, s.total-first)
		if first+n < s.total {
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/octo/hello/pulls/7/commits?per_page=100&page=%d>; rel="next"`, s.srv.URL, page+1))
		}
		_, _ = io.WriteString(w, commitsJSON(first, max(n, 0)))
	}))
	t.Cleanup(s.srv.Close)
	c, err := New(WithBaseURL(s.srv.URL), WithToken("t"), WithHTTPClient(s.srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

var octoHello = core.RepoRef{Owner: "octo", Name: "hello"}

func TestListPullRequestCommitsPages(t *testing.T) {
	s, c := newPullCommitsServer(t, 230)
	p, res, err := c.ListPullRequestCommits(t.Context(), octoHello, 7, Conditional{})
	if err != nil {
		t.Fatalf("ListPullRequestCommits: %v", err)
	}
	if len(p.Items) != 230 || p.Next != "" || p.Truncated {
		t.Fatalf("got %d commits, Next %q, Truncated %v; want all 230 in one page", len(p.Items), p.Next, p.Truncated)
	}
	if res.ETag != `W/"e1"` {
		t.Errorf("ETag = %q, want that of the first page", res.ETag)
	}
	if strings.Join(s.pages, ",") != "1,2,3" {
		t.Errorf("pages read = %v, want 1,2,3", s.pages)
	}
	first, last := p.Items[0], p.Items[229]
	if first.SHA != fmt.Sprintf("%040d", 0) || first.Subject != "commit 0" || first.Author.Login != "octocat" || first.Author.Date.IsZero() {
		t.Errorf("first commit = %+v, want the SHA, subject, login and date", first)
	}
	if last.Subject != "commit 229" {
		t.Errorf("last commit = %q, want commit 229: the order GitHub gives", last.Subject)
	}
}

func TestListPullRequestCommitsOnePage(t *testing.T) {
	s, c := newPullCommitsServer(t, 3)
	p, _, err := c.ListPullRequestCommits(t.Context(), octoHello, 7, Conditional{})
	if err != nil || len(p.Items) != 3 || p.Truncated {
		t.Fatalf("= %d commits, truncated %v, %v; want 3", len(p.Items), p.Truncated, err)
	}
	if len(s.pages) != 1 {
		t.Errorf("requests = %d, want 1 for a short list", len(s.pages))
	}
}

func TestListPullRequestCommitsCap(t *testing.T) {
	// GitHub lists 250 at most; a list that reaches that is Truncated, and
	// a page past it, if a server sent one, is not read.
	s, c := newPullCommitsServer(t, 400)
	p, _, err := c.ListPullRequestCommits(t.Context(), octoHello, 7, Conditional{})
	if err != nil {
		t.Fatalf("ListPullRequestCommits: %v", err)
	}
	if len(p.Items) != core.MaxPullCommits || !p.Truncated {
		t.Errorf("got %d commits, Truncated %v; want %d, truncated", len(p.Items), p.Truncated, core.MaxPullCommits)
	}
	if strings.Join(s.pages, ",") != "1,2,3" {
		t.Errorf("pages read = %v, want 1,2,3 and no more", s.pages)
	}
	if last := p.Items[len(p.Items)-1].Subject; last != "commit 249" {
		t.Errorf("last commit = %q, want commit 249", last)
	}
}

func TestListPullRequestCommitsConditional(t *testing.T) {
	s, c := newPullCommitsServer(t, 230)
	p, res, err := c.ListPullRequestCommits(t.Context(), octoHello, 7, Conditional{ETag: `W/"e1"`})
	if err != nil || !res.NotModified || len(p.Items) != 0 {
		t.Fatalf("= %+v, %+v, %v; want NotModified and no items", p, res, err)
	}
	if len(s.pages) != 1 {
		t.Errorf("requests = %d, want 1: a 304 on the first page ends the read", len(s.pages))
	}

	// Only the first page is conditional.
	s.conds = nil
	if _, _, err := c.ListPullRequestCommits(t.Context(), octoHello, 7, Conditional{ETag: `W/"old"`}); err != nil {
		t.Fatalf("ListPullRequestCommits: %v", err)
	}
	if len(s.conds) != 3 || s.conds[0] != `W/"old"` || s.conds[1] != "" || s.conds[2] != "" {
		t.Errorf("If-None-Match of the pages = %q, want the ETag on the first only", s.conds)
	}
}

func TestListPullRequestCommitsErrors(t *testing.T) {
	t.Run("first page", func(t *testing.T) {
		c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}))
		_, _, err := c.ListPullRequestCommits(t.Context(), octoHello, 7, Conditional{})
		if e, ok := errors.AsType[*Error](err); !ok || e.StatusCode != http.StatusNotFound {
			t.Errorf("err = %v, want a 404 Error", err)
		}
	})
	t.Run("later page", func(t *testing.T) {
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("page") != "" {
				http.Error(w, `{"message":"Server Error"}`, http.StatusUnprocessableEntity)
				return
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/octo/hello/pulls/7/commits?per_page=100&page=2>; rel="next"`, srv.URL))
			_, _ = io.WriteString(w, commitsJSON(0, 100))
		}))
		t.Cleanup(srv.Close)
		c, err := New(WithBaseURL(srv.URL), WithToken("t"), WithHTTPClient(srv.Client()))
		if err != nil {
			t.Fatal(err)
		}
		p, _, err := c.ListPullRequestCommits(t.Context(), octoHello, 7, Conditional{})
		if err == nil || len(p.Items) != 0 {
			t.Errorf("= %d commits, %v; want no partial list and an error", len(p.Items), err)
		}
	})
}
