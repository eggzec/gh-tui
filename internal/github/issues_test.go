package github

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

var issueRepo = core.RepoRef{Owner: "octo-org", Name: "hello"}

// serveIssueFixture answers every request with the named file in testdata,
// after check has looked at the request.
func serveIssueFixture(t *testing.T, name string, check func(r *http.Request)) *Client {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		check(r)
		w.Header().Set("ETag", `W/"e1"`)
		_, _ = w.Write(body)
	}))
}

func checkIssueRequest(t *testing.T, r *http.Request, method, path string, query map[string]string) {
	t.Helper()
	if r.Method != method || r.URL.Path != path {
		t.Errorf("request = %s %s, want %s %s", r.Method, r.URL.Path, method, path)
	}
	got := r.URL.Query()
	for k, want := range query {
		if got.Get(k) != want {
			t.Errorf("query %s = %q, want %q", k, got.Get(k), want)
		}
	}
	for k := range got {
		if _, ok := query[k]; !ok {
			t.Errorf("unexpected query parameter %s=%q", k, got.Get(k))
		}
	}
}

func issueTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

var wantIssue42 = core.Issue{
	ID:        "I_kwDOJ5Hs3c6t5a1K",
	Repo:      issueRepo,
	Number:    42,
	Title:     "Crash when the config file is empty",
	Body:      "Steps to reproduce:\n\n1. Create an empty config.yaml\n2. Run gh-tui",
	State:     core.StateOpen,
	Author:    core.User{Login: "octocat"},
	Labels:    []core.Label{{Name: "bug", Color: "d73a4a", Description: "Something isn't working"}},
	Assignees: []core.User{{Login: "hubot"}},
	Comments:  2,
	CreatedAt: issueTime("2026-09-01T08:15:00Z"),
	UpdatedAt: issueTime("2026-09-20T17:42:10Z"),
	URL:       "https://github.com/octo-org/hello/issues/42",
}

func equalIssue(a, b core.Issue) bool {
	return a.ID == b.ID && a.Repo == b.Repo && a.Number == b.Number &&
		a.Title == b.Title && a.Body == b.Body && a.State == b.State &&
		a.Author == b.Author && slices.Equal(a.Labels, b.Labels) &&
		slices.Equal(a.Assignees, b.Assignees) && a.Comments == b.Comments &&
		a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt) && a.URL == b.URL
}

func TestListIssues(t *testing.T) {
	tests := []struct {
		name  string
		state core.StateFilter
		query map[string]string
	}{
		{"closed", core.FilterClosed, map[string]string{"state": "closed"}},
		{"all", core.FilterAll, map[string]string{"state": "all"}},
		{"default", "", map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := map[string]string{"sort": "updated", "direction": "desc", "per_page": "50"}
			maps.Copy(query, tt.query)
			c := serveIssueFixture(t, "issues_list.json", func(r *http.Request) {
				checkIssueRequest(t, r, http.MethodGet, "/repos/octo-org/hello/issues", query)
			})

			page, res, err := c.ListIssues(t.Context(), issueRepo, tt.state, "", Conditional{})
			if err != nil {
				t.Fatalf("ListIssues: %v", err)
			}
			if res.ETag != `W/"e1"` {
				t.Errorf("ETag = %q, want the response's", res.ETag)
			}
			numbers := make([]int, len(page.Items))
			for i, it := range page.Items {
				numbers[i] = it.Number
			}
			if !slices.Equal(numbers, []int{42, 37}) {
				t.Fatalf("numbers = %v, want [42 37] without pull request 41", numbers)
			}
			if !equalIssue(page.Items[0], wantIssue42) {
				t.Errorf("issue = %+v, want %+v", page.Items[0], wantIssue42)
			}
			if b := page.Items[1]; b.Body != "" || b.Labels != nil || b.Assignees != nil {
				t.Errorf("issue 37 = %+v, want a null body and no labels or assignees", b)
			}
			if !page.Last() {
				t.Errorf("Next = %q, want the last page without a Link header", page.Next)
			}
		})
	}
}

func TestListIssuesPages(t *testing.T) {
	var queries []string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", `<http://`+r.Host+`/repositories/1/issues?page=2&per_page=50&state=open>; rel="next"`)
		}
		_, _ = w.Write([]byte(`[]`))
	}))

	first, _, err := c.ListIssues(t.Context(), issueRepo, core.FilterOpen, "", Conditional{})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if first.Last() || len(first.Items) != 0 {
		t.Fatalf("first page = %+v, want an empty page with a next cursor", first)
	}
	second, _, err := c.ListIssues(t.Context(), issueRepo, core.FilterOpen, first.Next, Conditional{})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if !second.Last() {
		t.Errorf("second page Next = %q, want empty", second.Next)
	}
	if len(queries) != 2 || queries[1] != "page=2&per_page=50&state=open" {
		t.Errorf("queries = %q, want the second to follow the Link header", queries)
	}
}

func TestGetIssue(t *testing.T) {
	c := serveIssueFixture(t, "issues_get.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/octo-org/hello/issues/42", nil)
	})
	got, res, err := c.GetIssue(t.Context(), issueRepo, 42, Conditional{})
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	want := wantIssue42
	want.Assignees = nil
	if !equalIssue(got, want) {
		t.Errorf("issue = %+v, want %+v", got, want)
	}
	if res.ETag != `W/"e1"` {
		t.Errorf("ETag = %q, want the response's", res.ETag)
	}
}

func TestListIssueComments(t *testing.T) {
	c := serveIssueFixture(t, "issues_comments.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/octo-org/hello/issues/42/comments", map[string]string{"per_page": "100"})
	})
	page, _, err := c.ListIssueComments(t.Context(), issueRepo, 42, "", Conditional{})
	if err != nil {
		t.Fatalf("ListIssueComments: %v", err)
	}
	want := []core.Comment{
		{
			ID:        "IC_kwDOJ5Hs3c7EwZ1R",
			Author:    core.User{Login: "hubot"},
			Body:      "I can reproduce this on v0.3.",
			CreatedAt: issueTime("2026-09-02T09:00:00Z"),
			UpdatedAt: issueTime("2026-09-02T09:00:00Z"),
		},
		{
			ID:        "IC_kwDOJ5Hs3c7EwZ2S",
			Author:    core.User{Login: "octocat"},
			Body:      "Fixed on main, thanks!",
			CreatedAt: issueTime("2026-09-20T17:42:10Z"),
			UpdatedAt: issueTime("2026-09-20T18:01:00Z"),
		},
	}
	if !slices.EqualFunc(page.Items, want, equalComment) {
		t.Errorf("comments = %+v, want %+v", page.Items, want)
	}
}

func equalComment(a, b core.Comment) bool {
	return a.ID == b.ID && a.Author == b.Author && a.Body == b.Body &&
		a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt)
}

// issueReads calls each read method with cond and reports whether the
// result was empty.
var issueReads = map[string]func(c *Client, ctx context.Context, cond Conditional) (Response, bool, error){
	"ListIssues": func(c *Client, ctx context.Context, cond Conditional) (Response, bool, error) {
		p, res, err := c.ListIssues(ctx, issueRepo, core.FilterOpen, "", cond)
		return res, p.Items == nil && p.Next == "", err
	},
	"GetIssue": func(c *Client, ctx context.Context, cond Conditional) (Response, bool, error) {
		it, res, err := c.GetIssue(ctx, issueRepo, 42, cond)
		return res, it.Number == 0, err
	},
	"ListIssueComments": func(c *Client, ctx context.Context, cond Conditional) (Response, bool, error) {
		p, res, err := c.ListIssueComments(ctx, issueRepo, 42, "", cond)
		return res, p.Items == nil && p.Next == "", err
	},
}

func TestIssueReadsNotModified(t *testing.T) {
	for name, read := range issueReads {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("If-None-Match") != `W/"e1"` {
					t.Errorf("If-None-Match = %q, want the stored ETag", r.Header.Get("If-None-Match"))
				}
				w.WriteHeader(http.StatusNotModified)
			}))
			res, empty, err := read(c, t.Context(), Conditional{ETag: `W/"e1"`})
			if err != nil {
				t.Fatalf("error = %v, want none on a 304", err)
			}
			if !res.NotModified || !empty {
				t.Errorf("NotModified = %v, empty = %v; want a 304 with no value", res.NotModified, empty)
			}
		})
	}
}

func TestIssueReadsErrors(t *testing.T) {
	statuses := map[int]error{
		http.StatusNotFound:     core.ErrNotFound,
		http.StatusUnauthorized: core.ErrUnauthorized,
	}
	for name, read := range issueReads {
		for status, want := range statuses {
			t.Run(name+"/"+http.StatusText(status), func(t *testing.T) {
				c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"message":"` + http.StatusText(status) + `"}`))
				}))
				_, empty, err := read(c, t.Context(), Conditional{})
				if !errors.Is(err, want) {
					t.Errorf("error = %v, want %v", err, want)
				}
				if !empty {
					t.Error("returned a value along with the error")
				}
			})
		}
	}
}
