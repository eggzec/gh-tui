package github

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
		a.Title == b.Title && a.Body == b.Body && a.State == b.State && a.Reason == b.Reason &&
		a.Author == b.Author && slices.Equal(a.Labels, b.Labels) &&
		slices.Equal(a.Assignees, b.Assignees) && a.Comments == b.Comments &&
		a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt) && a.URL == b.URL
}

func TestListIssues(t *testing.T) {
	tests := []struct {
		name    string
		state   core.StateFilter
		perPage int
		query   map[string]string
	}{
		{"closed", core.FilterClosed, 30, map[string]string{"state": "closed", "per_page": "30"}},
		{"all", core.FilterAll, 7, map[string]string{"state": "all", "per_page": "7"}},
		{"default", "", 0, map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := map[string]string{"sort": "updated", "direction": "desc"}
			maps.Copy(query, tt.query)
			c := serveIssueFixture(t, "issues_list.json", func(r *http.Request) {
				checkIssueRequest(t, r, http.MethodGet, "/repos/octo-org/hello/issues", query)
			})

			page, res, err := c.ListIssues(t.Context(), issueRepo, tt.state, "", tt.perPage, Conditional{})
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

func TestFilterIssues(t *testing.T) {
	f := IssueFilter{
		State: core.FilterAll, Labels: []string{"bug", "good first issue"}, Assignee: "none",
		Creator: "octocat", Mentioned: "hubot", Milestone: "3", Sort: "comments", Asc: true,
	}
	c := serveIssueFixture(t, "issues_list.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/octo-org/hello/issues", map[string]string{
			"state": "all", "labels": "bug,good first issue", "assignee": "none", "creator": "octocat",
			"mentioned": "hubot", "milestone": "3", "sort": "comments", "direction": "asc", "per_page": "30",
		})
	})
	page, _, err := c.FilterIssues(t.Context(), issueRepo, f, "", 30, Conditional{})
	if err != nil {
		t.Fatalf("FilterIssues: %v", err)
	}
	if len(page.Items) != 2 {
		t.Errorf("got %d issues, want 2", len(page.Items))
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

	first, _, err := c.ListIssues(t.Context(), issueRepo, core.FilterOpen, "", 50, Conditional{})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if first.Last() || len(first.Items) != 0 {
		t.Fatalf("first page = %+v, want an empty page with a next cursor", first)
	}
	// The cursor carries its page size, so the one given is not sent.
	second, _, err := c.ListIssues(t.Context(), issueRepo, core.FilterOpen, first.Next, 10, Conditional{})
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
		checkIssueRequest(t, r, http.MethodGet, "/repos/octo-org/hello/issues/42/comments", map[string]string{"per_page": "12"})
	})
	page, _, err := c.ListIssueComments(t.Context(), issueRepo, 42, "", 12, Conditional{})
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
	if !page.Last() {
		t.Errorf("Next = %q, want the last page without a Link header", page.Next)
	}
}

func TestListIssueCommentsPages(t *testing.T) {
	var queries []string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", `<http://`+r.Host+`/repositories/1/issues/42/comments?page=2&per_page=2>; rel="next"`)
		}
		_, _ = w.Write([]byte(`[]`))
	}))

	first, _, err := c.ListIssueComments(t.Context(), issueRepo, 42, "", 2, Conditional{})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if first.Last() {
		t.Fatalf("first page = %+v, want a next cursor", first)
	}
	second, _, err := c.ListIssueComments(t.Context(), issueRepo, 42, first.Next, 5, Conditional{})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if !second.Last() {
		t.Errorf("second page Next = %q, want empty", second.Next)
	}
	if !slices.Equal(queries, []string{"per_page=2", "page=2&per_page=2"}) {
		t.Errorf("queries = %q, want the second to follow the Link header", queries)
	}
}

func TestListIssueCommentsDefaultSize(t *testing.T) {
	c := serveIssueFixture(t, "issues_comments.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/octo-org/hello/issues/42/comments", nil)
	})
	if _, _, err := c.ListIssueComments(t.Context(), issueRepo, 42, "", 0, Conditional{}); err != nil {
		t.Fatalf("ListIssueComments: %v", err)
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
		p, res, err := c.ListIssues(ctx, issueRepo, core.FilterOpen, "", 30, cond)
		return res, p.Items == nil && p.Next == "", err
	},
	"GetIssue": func(c *Client, ctx context.Context, cond Conditional) (Response, bool, error) {
		it, res, err := c.GetIssue(ctx, issueRepo, 42, cond)
		return res, it.Number == 0, err
	},
	"ListIssueComments": func(c *Client, ctx context.Context, cond Conditional) (Response, bool, error) {
		p, res, err := c.ListIssueComments(ctx, issueRepo, 42, "", 30, cond)
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

// serveIssueMutation answers every request with status and the named file
// in testdata, after check has looked at the request and its JSON body.
func serveIssueMutation(t *testing.T, name string, status int, check func(r *http.Request, body string)) *Client {
	t.Helper()
	out, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		in, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		check(r, strings.TrimSpace(string(in)))
		w.WriteHeader(status)
		_, _ = w.Write(out)
	}))
}

func checkIssueBody(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("request body = %s, want %s", got, want)
	}
}

func TestSetIssueState(t *testing.T) {
	c := serveIssueMutation(t, "issues_update.json", http.StatusOK, func(r *http.Request, body string) {
		checkIssueRequest(t, r, http.MethodPatch, "/repos/octo-org/hello/issues/42", nil)
		checkIssueBody(t, body, `{"state":"closed"}`)
	})
	got, err := c.SetIssueState(t.Context(), issueRepo, 42, core.StateClosed)
	if err != nil {
		t.Fatalf("SetIssueState: %v", err)
	}
	want := wantIssue42
	want.State = core.StateClosed
	want.Reason = core.ReasonCompleted
	want.Assignees = nil
	want.UpdatedAt = issueTime("2026-09-21T10:00:00Z")
	if !equalIssue(got, want) {
		t.Errorf("issue = %+v, want %+v", got, want)
	}
}

var wantIssueLabels = []core.Label{
	{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
	{Name: "good first issue", Color: "7057ff", Description: "Good for newcomers"},
}

func TestAddIssueLabels(t *testing.T) {
	c := serveIssueMutation(t, "issues_labels.json", http.StatusOK, func(r *http.Request, body string) {
		checkIssueRequest(t, r, http.MethodPost, "/repos/octo-org/hello/issues/42/labels", nil)
		checkIssueBody(t, body, `{"labels":["good first issue"]}`)
	})
	got, err := c.AddIssueLabels(t.Context(), issueRepo, 42, []string{"good first issue"})
	if err != nil {
		t.Fatalf("AddIssueLabels: %v", err)
	}
	if !slices.Equal(got, wantIssueLabels) {
		t.Errorf("labels = %+v, want %+v", got, wantIssueLabels)
	}
}

func TestRemoveIssueLabel(t *testing.T) {
	c := serveIssueMutation(t, "issues_labels.json", http.StatusOK, func(r *http.Request, body string) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		// The name is one path segment, so its slash must stay escaped.
		if got, want := r.URL.EscapedPath(), "/repos/octo-org/hello/issues/42/labels/area%2Fui%20kit"; got != want {
			t.Errorf("path = %s, want %s", got, want)
		}
		checkIssueBody(t, body, "")
	})
	got, err := c.RemoveIssueLabel(t.Context(), issueRepo, 42, "area/ui kit")
	if err != nil {
		t.Fatalf("RemoveIssueLabel: %v", err)
	}
	if !slices.Equal(got, wantIssueLabels) {
		t.Errorf("labels = %+v, want %+v", got, wantIssueLabels)
	}
}

func TestRemoveLastIssueLabel(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	got, err := c.RemoveIssueLabel(t.Context(), issueRepo, 42, "bug")
	if err != nil || got != nil {
		t.Errorf("RemoveIssueLabel = %v, %v; want no labels", got, err)
	}
}

func TestCreateIssueComment(t *testing.T) {
	c := serveIssueMutation(t, "issues_comment_created.json", http.StatusCreated, func(r *http.Request, body string) {
		checkIssueRequest(t, r, http.MethodPost, "/repos/octo-org/hello/issues/42/comments", nil)
		checkIssueBody(t, body, `{"body":"Closing, fixed in v0.4."}`)
	})
	got, err := c.CreateIssueComment(t.Context(), issueRepo, 42, "Closing, fixed in v0.4.")
	if err != nil {
		t.Fatalf("CreateIssueComment: %v", err)
	}
	want := core.Comment{
		ID:        "IC_kwDOJ5Hs3c7EwZ3T",
		Author:    core.User{Login: "octocat"},
		Body:      "Closing, fixed in v0.4.",
		CreatedAt: issueTime("2026-09-21T10:05:00Z"),
		UpdatedAt: issueTime("2026-09-21T10:05:00Z"),
	}
	if !equalComment(got, want) {
		t.Errorf("comment = %+v, want %+v", got, want)
	}
}

func TestIssueMutationErrors(t *testing.T) {
	mutations := map[string]func(c *Client, ctx context.Context) error{
		"SetIssueState": func(c *Client, ctx context.Context) error {
			_, err := c.SetIssueState(ctx, issueRepo, 42, core.StateClosed)
			return err
		},
		"AddIssueLabels": func(c *Client, ctx context.Context) error {
			_, err := c.AddIssueLabels(ctx, issueRepo, 42, []string{"bug"})
			return err
		},
		"RemoveIssueLabel": func(c *Client, ctx context.Context) error {
			_, err := c.RemoveIssueLabel(ctx, issueRepo, 42, "bug")
			return err
		},
		"CreateIssueComment": func(c *Client, ctx context.Context) error {
			_, err := c.CreateIssueComment(ctx, issueRepo, 42, "")
			return err
		},
	}
	statuses := map[int]error{
		http.StatusNotFound:            core.ErrNotFound,
		http.StatusUnprocessableEntity: core.ErrConflict,
	}
	for name, mutate := range mutations {
		for status, want := range statuses {
			t.Run(name+"/"+http.StatusText(status), func(t *testing.T) {
				c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"message":"Validation Failed","errors":[{"resource":"IssueComment","code":"missing_field","field":"body"}]}`))
				}))
				if err := mutate(c, t.Context()); !errors.Is(err, want) {
					t.Errorf("error = %v, want %v", err, want)
				}
			})
		}
	}
}
