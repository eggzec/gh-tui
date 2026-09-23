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

	page, err := c.ListPullRequests(t.Context(), pullsRepo, core.StateOpen, "")
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	checkPullQuery(t, reqs(), "pullRequests(states: $states, first: $first, after: $after, orderBy: {field: UPDATED_AT, direction: DESC})", map[string]any{
		"owner": "eggzec", "name": "gh-tui", "states": []any{"OPEN"}, "first": float64(pullPageSize),
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

func TestListPullRequestsVariables(t *testing.T) {
	tests := []struct {
		name   string
		state  core.State
		cursor string
		want   map[string]any
	}{
		{"all states", "", "", map[string]any{"states": nil}},
		{"merged", core.StateMerged, "", map[string]any{"states": []any{"MERGED"}}},
		{"closed after cursor", core.StateClosed, "abc", map[string]any{"states": []any{"CLOSED"}, "after": "abc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := pullServer(t, "pulls_list.json")
			if _, err := c.ListPullRequests(t.Context(), pullsRepo, tt.state, tt.cursor); err != nil {
				t.Fatalf("ListPullRequests: %v", err)
			}
			want := map[string]any{"owner": "eggzec", "name": "gh-tui", "first": float64(pullPageSize)}
			maps.Copy(want, tt.want)
			checkPullQuery(t, reqs(), "pullRequests(", want)
		})
	}
}

func TestListPullRequestsUnknownState(t *testing.T) {
	c, reqs := pullServer(t, "pulls_list.json")
	if _, err := c.ListPullRequests(t.Context(), pullsRepo, "draft", ""); err == nil {
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

	if got.Number != 42 || got.Body != "Lists pull requests with their checks.\r\n\r\nCloses #40." {
		t.Errorf("number %d, body %q; want 42 and the body", got.Number, got.Body)
	}
	if got.ReviewDecision != core.ReviewRequired || got.Checks != core.ChecksPending || got.Comments != 2 {
		t.Errorf("review %q, checks %q, comments %d; want review_required, pending, 2",
			got.ReviewDecision, got.Checks, got.Comments)
	}
	wantReviews := []core.Review{
		{ID: "PRR_kwDOLnBTf86Aa001", Author: core.User{Login: "hubot", Name: "Hubot"}, State: core.ReviewStateChangesRequested, Body: "Please add tests.", SubmittedAt: pullTime("2026-09-21T10:00:00Z")},
		{ID: "PRR_kwDOLnBTf86Aa002", Author: core.User{Login: "monalisa", Name: "Mona Lisa"}, State: core.ReviewStateCommented, SubmittedAt: pullTime("2026-09-22T09:12:30Z")},
	}
	if !reflect.DeepEqual(got.Reviews, wantReviews) {
		t.Errorf("reviews =\n%+v\nwant\n%+v", got.Reviews, wantReviews)
	}
	wantComments := []core.Comment{
		{ID: "IC_kwDOLnBTf86Bb001", Author: core.User{Login: "monalisa", Name: "Mona Lisa"}, Body: "Looks good so far.", CreatedAt: pullTime("2026-09-21T08:00:00Z"), UpdatedAt: pullTime("2026-09-21T08:05:00Z")},
		{ID: "IC_kwDOLnBTf86Bb002", Body: "Ping.", CreatedAt: pullTime("2026-09-22T16:00:00Z"), UpdatedAt: pullTime("2026-09-22T16:00:00Z")},
	}
	if !reflect.DeepEqual(got.RecentComments, wantComments) {
		t.Errorf("comments =\n%+v\nwant\n%+v", got.RecentComments, wantComments)
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
			_, err := c.ListPullRequests(t.Context(), pullsRepo, core.StateOpen, "")
			return err
		},
		"get": func(c *Client) error {
			_, err := c.GetPullRequest(t.Context(), pullsRepo, 42)
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

func TestGetPullRequestNullPull(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"repository":{"pullQuery":null}},"errors":[{"type":"NOT_FOUND","path":["repository","pullQuery"],"message":"Could not resolve to a PullRequest with the number of 999."}]}`))
	}))
	if _, err := c.GetPullRequest(t.Context(), pullsRepo, 999); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}
