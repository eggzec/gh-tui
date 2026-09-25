package github

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

var pullsRepo = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}

// pullQuery is a GraphQL request as the test server received it.
type pullQuery struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// pullServer answers every GraphQL request with the fixture in testdata. It
// returns the client and a function that returns the requests received so
// far.
func pullServer(t *testing.T, fixture string) (c *Client, requests func() []pullQuery) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	var (
		mu   sync.Mutex
		reqs []pullQuery
	)
	c = newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			t.Errorf("request = %s %s, want POST /graphql", r.Method, r.URL.Path)
		}
		var req pullQuery
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		reqs = append(reqs, req)
		mu.Unlock()
		_, _ = w.Write(body)
	}))
	return c, func() []pullQuery {
		mu.Lock()
		defer mu.Unlock()
		return reqs
	}
}

// checkPullQuery reports whether the only request had the operation and the
// variables want. JSON numbers decode as float64.
func checkPullQuery(t *testing.T, reqs []pullQuery, operation string, want map[string]any) {
	t.Helper()
	if len(reqs) != 1 {
		t.Fatalf("got %d requests, want 1", len(reqs))
	}
	if !strings.Contains(reqs[0].Query, operation) {
		t.Errorf("query does not contain %q:\n%s", operation, reqs[0].Query)
	}
	if !reflect.DeepEqual(reqs[0].Variables, want) {
		t.Errorf("variables = %#v, want %#v", reqs[0].Variables, want)
	}
}

func pullTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestListPullRequests(t *testing.T) {
	c, reqs := pullServer(t, "pulls_list.json")

	page, err := c.ListPullRequests(t.Context(), pullsRepo, core.StateOpen, "", 30)
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	checkPullQuery(t, reqs(), "pullRequests(states: $states, labels: $labels, baseRefName: $base, headRefName: $head, orderBy: $order,", map[string]any{
		"owner": "eggzec", "name": "gh-tui", "states": []any{"OPEN"}, "first": float64(30), "order": updatedDesc,
	})

	if page.Next != "Y3Vyc29yOnYyOpK5MjAyNi0wOS0yMFQxMjowMDowMFo" {
		t.Errorf("next = %q, want the end cursor", page.Next)
	}
	if len(page.Items) != 3 {
		t.Fatalf("got %d pull requests, want 3", len(page.Items))
	}
	want := core.PullRequest{
		ID:             "PR_kwDOLnBTf85xYz01",
		Repo:           pullsRepo,
		Number:         42,
		Title:          "Add pull request list",
		State:          core.StateOpen,
		Author:         core.User{Login: "octocat", Name: "The Octocat"},
		Labels:         []core.Label{{Name: "enhancement", Color: "a2eeef", Description: "New feature or request"}},
		Assignees:      []core.User{{Login: "hubot", Name: "Hubot"}},
		Comments:       3,
		CreatedAt:      pullTime("2026-09-20T08:15:00Z"),
		UpdatedAt:      pullTime("2026-09-22T17:40:12Z"),
		URL:            "https://github.com/eggzec/gh-tui/pull/42",
		HeadRef:        "feat/pulls",
		BaseRef:        "main",
		ReviewDecision: core.ReviewApproved,
		Checks:         core.ChecksSuccess,
		Additions:      512,
		Deletions:      18,
		ChangedFiles:   7,
	}
	if !reflect.DeepEqual(page.Items[0], want) {
		t.Errorf("first pull request =\n%+v\nwant\n%+v", page.Items[0], want)
	}

	draft := page.Items[1]
	if !draft.Draft || draft.ReviewDecision != core.ReviewChangesRequested || draft.Checks != core.ChecksFailure {
		t.Errorf("draft = %v, review %q, checks %q; want draft, changes_requested, failure",
			draft.Draft, draft.ReviewDecision, draft.Checks)
	}
	if draft.Author != (core.User{Login: "dependabot"}) || draft.Labels != nil {
		t.Errorf("bot author = %+v, labels %v; want login only and no labels", draft.Author, draft.Labels)
	}

	ghost := page.Items[2]
	if ghost.Author != (core.User{}) || ghost.ReviewDecision != core.ReviewNone || ghost.Checks != core.ChecksNone {
		t.Errorf("author %+v, review %q, checks %q; want zero values for nulls", ghost.Author, ghost.ReviewDecision, ghost.Checks)
	}
}

// updatedDesc is the order of a list without a sort, as the test server
// receives it.
var updatedDesc = map[string]any{"field": "UPDATED_AT", "direction": "DESC"}

func TestListPullRequestsVariables(t *testing.T) {
	tests := []struct {
		name   string
		state  core.State
		cursor string
		first  int
		want   map[string]any
	}{
		{"all states", "", "", 30, map[string]any{"states": nil, "first": float64(30)}},
		{"merged", core.StateMerged, "", 30, map[string]any{"states": []any{"MERGED"}, "first": float64(30)}},
		{"closed after cursor", core.StateClosed, "abc", 30, map[string]any{"states": []any{"CLOSED"}, "first": float64(30), "after": "abc"}},
		{"page size", core.StateOpen, "abc", 7, map[string]any{"states": []any{"OPEN"}, "first": float64(7), "after": "abc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := pullServer(t, "pulls_list.json")
			if _, err := c.ListPullRequests(t.Context(), pullsRepo, tt.state, tt.cursor, tt.first); err != nil {
				t.Fatalf("ListPullRequests: %v", err)
			}
			want := map[string]any{"owner": "eggzec", "name": "gh-tui", "order": updatedDesc}
			maps.Copy(want, tt.want)
			checkPullQuery(t, reqs(), "pullRequests(", want)
		})
	}
}

func TestFilterPullRequestsVariables(t *testing.T) {
	tests := []struct {
		name   string
		filter PullFilter
		want   map[string]any
	}{
		{"label", PullFilter{State: core.StateOpen, Labels: []string{"bug"}},
			map[string]any{"states": []any{"OPEN"}, "labels": []any{"bug"}, "order": updatedDesc}},
		{"branches", PullFilter{Base: "main", Head: "feat/x"},
			map[string]any{"states": nil, "base": "main", "head": "feat/x", "order": updatedDesc}},
		{"created ascending", PullFilter{State: core.StateMerged, Sort: "created", Asc: true},
			map[string]any{"states": []any{"MERGED"}, "order": map[string]any{"field": "CREATED_AT", "direction": "ASC"}}},
		{"most commented", PullFilter{Sort: "comments"},
			map[string]any{"states": nil, "order": map[string]any{"field": "COMMENTS", "direction": "DESC"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := pullServer(t, "pulls_list.json")
			if _, err := c.FilterPullRequests(t.Context(), pullsRepo, tt.filter, "", 30); err != nil {
				t.Fatalf("FilterPullRequests: %v", err)
			}
			want := map[string]any{"owner": "eggzec", "name": "gh-tui", "first": float64(30)}
			maps.Copy(want, tt.want)
			checkPullQuery(t, reqs(), "pullRequests(", want)
		})
	}
}

func TestFilterPullRequestsUnknownSort(t *testing.T) {
	c, reqs := pullServer(t, "pulls_list.json")
	if _, err := c.FilterPullRequests(t.Context(), pullsRepo, PullFilter{Sort: "reactions"}, "", 30); err == nil {
		t.Error("FilterPullRequests with an unknown sort succeeded")
	}
	if n := len(reqs()); n != 0 {
		t.Errorf("sent %d requests, want none", n)
	}
}

func TestSearchPullRequests(t *testing.T) {
	c, reqs := pullServer(t, "pulls_search.json")
	const q = "repo:charmbracelet/bubbletea is:pr is:open review:approved sort:updated-desc"
	page, err := c.SearchPullRequests(t.Context(), q, "abc", 2)
	if err != nil {
		t.Fatalf("SearchPullRequests: %v", err)
	}
	checkPullQuery(t, reqs(), "search(type: ISSUE", map[string]any{"query": q, "first": float64(2), "after": "abc"})
	if len(page.Items) != 2 {
		t.Fatalf("got %d pull requests, want 2 without the issue", len(page.Items))
	}
	pr := page.Items[0]
	if pr.Number != 1600 || pr.Repo != (core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}) || pr.State != core.StateOpen || pr.Author.Login == "" {
		t.Errorf("first = %+v, want #1600 of charmbracelet/bubbletea, open, with its author", pr)
	}
	if page.Next != "Y3Vyc29yOjI=" {
		t.Errorf("next = %q, want the end cursor", page.Next)
	}
}

func TestListPullRequestsUnknownState(t *testing.T) {
	c, reqs := pullServer(t, "pulls_list.json")
	if _, err := c.ListPullRequests(t.Context(), pullsRepo, "draft", "", 30); err == nil {
		t.Error("ListPullRequests with an unknown state succeeded")
	}
	if n := len(reqs()); n != 0 {
		t.Errorf("sent %d requests, want none", n)
	}
}

func TestGetPullRequest(t *testing.T) {
	c, reqs := pullServer(t, "pulls_detail.json")

	got, err := c.GetPullRequest(t.Context(), pullsRepo, 42)
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	checkPullQuery(t, reqs(), "pullRequest(number: $number)", map[string]any{
		"owner": "eggzec", "name": "gh-tui", "number": float64(42),
	})
	// Reviews and comments are paged on their own; the detail counts the
	// comments only.
	for _, field := range []string{"reviews", "comments(", "recentComments"} {
		if strings.Contains(reqs()[0].Query, field) {
			t.Errorf("detail query selects %q:\n%s", field, reqs()[0].Query)
		}
	}

	if got.Number != 42 || got.Body != "Lists pull requests with their checks.\r\n\r\nCloses #40." {
		t.Errorf("number %d, body %q; want 42 and the body", got.Number, got.Body)
	}
	if got.ReviewDecision != core.ReviewRequired || got.Checks != core.ChecksPending || got.Comments != 2 {
		t.Errorf("review %q, checks %q, comments %d; want review_required, pending, 2",
			got.ReviewDecision, got.Checks, got.Comments)
	}
	wantChecks := []core.CheckRun{
		{Name: "test (ubuntu-latest)", Status: "completed", Conclusion: "success", URL: "https://github.com/eggzec/gh-tui/actions/runs/1001/job/2001"},
		{Name: "lint", Status: "in_progress", URL: "https://github.com/eggzec/gh-tui/actions/runs/1001/job/2002"},
		{Name: "ci/coverage", Status: "pending", URL: "https://coverage.example.com/eggzec/gh-tui/42"},
		{Name: "license/cla", Status: "completed", Conclusion: "success", URL: "https://cla.example.com/eggzec/gh-tui"},
	}
	if !reflect.DeepEqual(got.CheckRuns, wantChecks) {
		t.Errorf("check runs =\n%+v\nwant\n%+v", got.CheckRuns, wantChecks)
	}
}

func TestListPullRequestComments(t *testing.T) {
	c, reqs := pullServer(t, "pulls_comments.json")

	page, err := c.ListPullRequestComments(t.Context(), pullsRepo, 42, "", 30)
	if err != nil {
		t.Fatalf("ListPullRequestComments: %v", err)
	}
	checkPullQuery(t, reqs(), "page: comments(first: $first, after: $after)", map[string]any{
		"owner": "eggzec", "name": "gh-tui", "number": float64(42), "first": float64(30),
	})
	want := core.Page[core.Comment]{
		Items: []core.Comment{
			{ID: "IC_kwDOLnBTf86Bb001", Author: core.User{Login: "monalisa", Name: "Mona Lisa"}, Body: "Looks good so far.", CreatedAt: pullTime("2026-09-21T08:00:00Z"), UpdatedAt: pullTime("2026-09-21T08:05:00Z")},
			{ID: "IC_kwDOLnBTf86Bb002", Body: "Ping.", CreatedAt: pullTime("2026-09-22T16:00:00Z"), UpdatedAt: pullTime("2026-09-22T16:00:00Z")},
		},
		Next: "Y3Vyc29yOnYyOpHOBb002",
	}
	if !reflect.DeepEqual(page, want) {
		t.Errorf("page =\n%+v\nwant\n%+v", page, want)
	}
}

func TestListPullRequestReviews(t *testing.T) {
	c, reqs := pullServer(t, "pulls_reviews.json")

	page, err := c.ListPullRequestReviews(t.Context(), pullsRepo, 42, "", 30)
	if err != nil {
		t.Fatalf("ListPullRequestReviews: %v", err)
	}
	checkPullQuery(t, reqs(), "page: reviews(first: $first, after: $after)", map[string]any{
		"owner": "eggzec", "name": "gh-tui", "number": float64(42), "first": float64(30),
	})
	want := core.Page[core.Review]{
		Items: []core.Review{
			{ID: "PRR_kwDOLnBTf86Aa001", Author: core.User{Login: "hubot", Name: "Hubot"}, State: core.ReviewStateChangesRequested, Body: "Please add tests.", SubmittedAt: pullTime("2026-09-21T10:00:00Z")},
			{ID: "PRR_kwDOLnBTf86Aa002", Author: core.User{Login: "monalisa", Name: "Mona Lisa"}, State: core.ReviewStateCommented, SubmittedAt: pullTime("2026-09-22T09:12:30Z")},
		},
	}
	if !reflect.DeepEqual(page, want) {
		t.Errorf("page =\n%+v\nwant the last page\n%+v", page, want)
	}
}

func TestPullPageVariables(t *testing.T) {
	reads := map[string]struct {
		fixture string
		read    func(c *Client, cursor string, first int) error
	}{
		"comments": {"pulls_comments.json", func(c *Client, cursor string, first int) error {
			_, err := c.ListPullRequestComments(t.Context(), pullsRepo, 42, cursor, first)
			return err
		}},
		"reviews": {"pulls_reviews.json", func(c *Client, cursor string, first int) error {
			_, err := c.ListPullRequestReviews(t.Context(), pullsRepo, 42, cursor, first)
			return err
		}},
	}
	pages := []struct {
		name   string
		cursor string
		first  int
		want   map[string]any
	}{
		{"first page", "", 30, map[string]any{"first": float64(30)}},
		{"after cursor", "abc", 30, map[string]any{"first": float64(30), "after": "abc"}},
		{"page size", "abc", 7, map[string]any{"first": float64(7), "after": "abc"}},
	}
	for name, r := range reads {
		for _, p := range pages {
			t.Run(name+"/"+p.name, func(t *testing.T) {
				c, reqs := pullServer(t, r.fixture)
				if err := r.read(c, p.cursor, p.first); err != nil {
					t.Fatalf("read: %v", err)
				}
				want := map[string]any{"owner": "eggzec", "name": "gh-tui", "number": float64(42)}
				maps.Copy(want, p.want)
				checkPullQuery(t, reqs(), name+"(first: $first, after: $after)", want)
			})
		}
	}
}

func TestChecksState(t *testing.T) {
	tests := map[string]core.ChecksState{
		"SUCCESS":  core.ChecksSuccess,
		"PENDING":  core.ChecksPending,
		"EXPECTED": core.ChecksPending,
		"FAILURE":  core.ChecksFailure,
		"ERROR":    core.ChecksFailure,
		"":         core.ChecksNone,
	}
	for in, want := range tests {
		if got := checksState(in); got != want {
			t.Errorf("checksState(%q) = %q, want %q", in, got, want)
		}
	}
}

// pullNotFound is GitHub's answer for a missing repository or pull request.
const pullNotFound = `{
  "data": {"repository": null},
  "errors": [{"type": "NOT_FOUND", "path": ["repository"], "message": "Could not resolve to a Repository with the name 'eggzec/nope'."}]
}`

func TestPullReadErrors(t *testing.T) {
	reads := map[string]func(*Client) error{
		"list": func(c *Client) error {
			_, err := c.ListPullRequests(t.Context(), pullsRepo, core.StateOpen, "", 30)
			return err
		},
		"get": func(c *Client) error {
			_, err := c.GetPullRequest(t.Context(), pullsRepo, 42)
			return err
		},
		"comments": func(c *Client) error {
			_, err := c.ListPullRequestComments(t.Context(), pullsRepo, 42, "", 30)
			return err
		},
		"reviews": func(c *Client) error {
			_, err := c.ListPullRequestReviews(t.Context(), pullsRepo, 42, "", 30)
			return err
		},
	}
	responses := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"not found", http.StatusOK, pullNotFound, core.ErrNotFound},
		{"null repository", http.StatusOK, `{"data":{"repository":null}}`, core.ErrNotFound},
		{"unauthorized", http.StatusUnauthorized, `{"message":"Bad credentials"}`, core.ErrUnauthorized},
	}
	for name, read := range reads {
		for _, resp := range responses {
			t.Run(name+"/"+resp.name, func(t *testing.T) {
				c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(resp.status)
					_, _ = w.Write([]byte(resp.body))
				}))
				err := read(c)
				if !errors.Is(err, resp.want) {
					t.Errorf("error = %v, want %v", err, resp.want)
				}
				if !strings.Contains(err.Error(), "eggzec/gh-tui") {
					t.Errorf("error %q does not name the repository", err)
				}
			})
		}
	}
}

func TestPullReadsNullPull(t *testing.T) {
	reads := map[string]func(*Client) error{
		"get": func(c *Client) error {
			_, err := c.GetPullRequest(t.Context(), pullsRepo, 999)
			return err
		},
		"comments": func(c *Client) error {
			_, err := c.ListPullRequestComments(t.Context(), pullsRepo, 999, "", 30)
			return err
		},
		"reviews": func(c *Client) error {
			_, err := c.ListPullRequestReviews(t.Context(), pullsRepo, 999, "", 30)
			return err
		},
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":null}}}`))
			}))
			if err := read(c); !errors.Is(err, core.ErrNotFound) {
				t.Errorf("error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestPullRequestID(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req pullQuery
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		want := map[string]any{"owner": "eggzec", "name": "gh-tui", "number": float64(42)}
		if !strings.Contains(req.Query, "pullRequest(number: $number) { id }") || !reflect.DeepEqual(req.Variables, want) {
			t.Errorf("request = %+v, want the ID of #42", req)
		}
		_, _ = w.Write([]byte(`{"data":{"repository":{"pullRequest":{"id":"PR_kwDOLnBTf85xYz01"}}}}`))
	}))
	id, err := c.PullRequestID(t.Context(), pullsRepo, 42)
	if err != nil || id != "PR_kwDOLnBTf85xYz01" {
		t.Errorf("PullRequestID = %q, %v; want PR_kwDOLnBTf85xYz01", id, err)
	}
}

func TestPullRequestIDNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(pullNotFound))
	}))
	if _, err := c.PullRequestID(t.Context(), pullsRepo, 42); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestPullMutations(t *testing.T) {
	const id = "PR_kwDOLnBTf85xYz01"
	tests := []struct {
		name     string
		fixture  string
		call     func(*Client) (core.PullRequest, error)
		mutation string
		vars     map[string]any
		check    func(core.PullRequest) bool
	}{
		{
			name:    "merge",
			fixture: "pulls_merge.json",
			call: func(c *Client) (core.PullRequest, error) {
				return c.MergePullRequest(t.Context(), id, core.MergeSquash)
			},
			mutation: "mergePullRequest(input: {pullRequestId: $id, mergeMethod: $method})",
			vars:     map[string]any{"id": id, "method": "SQUASH"},
			check: func(pr core.PullRequest) bool {
				return pr.State == core.StateMerged && pr.MergedAt.Equal(pullTime("2026-09-23T09:00:04Z"))
			},
		},
		{
			name:     "close",
			fixture:  "pulls_close.json",
			call:     func(c *Client) (core.PullRequest, error) { return c.ClosePullRequest(t.Context(), id) },
			mutation: "closePullRequest(input: {pullRequestId: $id})",
			vars:     map[string]any{"id": id},
			check:    func(pr core.PullRequest) bool { return pr.State == core.StateClosed && pr.MergedAt.IsZero() },
		},
		{
			name:     "reopen",
			fixture:  "pulls_reopen.json",
			call:     func(c *Client) (core.PullRequest, error) { return c.ReopenPullRequest(t.Context(), id) },
			mutation: "reopenPullRequest(input: {pullRequestId: $id})",
			vars:     map[string]any{"id": id},
			check:    func(pr core.PullRequest) bool { return pr.State == core.StateOpen },
		},
		{
			name:     "mark ready",
			fixture:  "pulls_ready.json",
			call:     func(c *Client) (core.PullRequest, error) { return c.MarkPullRequestReady(t.Context(), id) },
			mutation: "markPullRequestReadyForReview(input: {pullRequestId: $id})",
			vars:     map[string]any{"id": id},
			check:    func(pr core.PullRequest) bool { return !pr.Draft },
		},
		{
			name:     "convert to draft",
			fixture:  "pulls_draft.json",
			call:     func(c *Client) (core.PullRequest, error) { return c.ConvertPullRequestToDraft(t.Context(), id) },
			mutation: "convertPullRequestToDraft(input: {pullRequestId: $id})",
			vars:     map[string]any{"id": id},
			check:    func(pr core.PullRequest) bool { return pr.Draft },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := pullServer(t, tt.fixture)
			pr, err := tt.call(c)
			if err != nil {
				t.Fatalf("mutation: %v", err)
			}
			checkPullQuery(t, reqs(), tt.mutation, tt.vars)
			if !strings.HasPrefix(reqs()[0].Query, "mutation ") {
				t.Errorf("query is not a mutation:\n%s", reqs()[0].Query)
			}
			if pr.ID != id || pr.Repo != pullsRepo || pr.Number != 42 || pr.Checks != core.ChecksSuccess || pr.ReviewDecision != core.ReviewApproved {
				t.Errorf("pull request = %+v, want #42 of %s decoded in full", pr, pullsRepo)
			}
			if !pr.UpdatedAt.Equal(pullTime("2026-09-23T09:00:05Z")) {
				t.Errorf("updated at = %v, want the server's time", pr.UpdatedAt)
			}
			if !tt.check(pr) {
				t.Errorf("pull request = %+v, want the mutation's result", pr)
			}
		})
	}
}

func TestMergePullRequestUnknownMethod(t *testing.T) {
	c, reqs := pullServer(t, "pulls_merge.json")
	if _, err := c.MergePullRequest(t.Context(), "PR_1", "fast-forward"); err == nil {
		t.Error("merge with an unknown method succeeded")
	}
	if n := len(reqs()); n != 0 {
		t.Errorf("sent %d requests, want none", n)
	}
}

func TestPullMutationErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{
			name:   "not mergeable",
			status: http.StatusOK,
			body:   `{"data":{"result":null},"errors":[{"type":"UNPROCESSABLE","path":["result"],"message":"Pull Request is not mergeable"}]}`,
			check: func(err error) bool {
				e, ok := errors.AsType[*GraphQLError](err)
				return ok && strings.Contains(e.Error(), "Pull Request is not mergeable")
			},
		},
		{
			name:   "unknown node",
			status: http.StatusOK,
			body:   `{"data":{"result":null},"errors":[{"type":"NOT_FOUND","path":["result"],"message":"Could not resolve to a node with the global id of 'PR_1'"}]}`,
			check:  func(err error) bool { return errors.Is(err, core.ErrNotFound) },
		},
		{
			name:   "no pull request",
			status: http.StatusOK,
			body:   `{"data":{"result":{"pullRequest":null}}}`,
			check:  func(err error) bool { return errors.Is(err, errNoPull) },
		},
		{
			name:   "unauthorized",
			status: http.StatusUnauthorized,
			body:   `{"message":"Bad credentials"}`,
			check:  func(err error) bool { return errors.Is(err, core.ErrUnauthorized) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			_, err := c.MergePullRequest(t.Context(), "PR_1", core.MergeCommit)
			if err == nil || !tt.check(err) {
				t.Errorf("error = %v, want %s", err, tt.name)
			}
			if err != nil && !strings.Contains(err.Error(), "merge pull request PR_1") {
				t.Errorf("error %q lacks context", err)
			}
		})
	}
}
