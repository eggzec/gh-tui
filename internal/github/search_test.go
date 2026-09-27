package github

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestSearchRepos(t *testing.T) {
	c := serveIssueFixture(t, "search_repos.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/search/repositories", map[string]string{"q": "tui user:eggzec", "per_page": "20"})
	})
	page, err := c.SearchRepos(t.Context(), "tui user:eggzec", "", 20)
	if err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	want := []core.Repo{
		{
			ID:            "R_kgDOJ5Hs3Q",
			Ref:           core.RepoRef{Owner: "eggzec", Name: "gh-tui"},
			Description:   "A GitHub client for the terminal",
			DefaultBranch: "main",
			Language:      "Go",
			Stars:         42,
			UpdatedAt:     issueTime("2026-09-22T12:30:00Z"),
			URL:           "https://github.com/eggzec/gh-tui",
		},
		{
			ID:            "MDEwOlJlcG9zaXRvcnkxMjk2MjY5",
			Ref:           core.RepoRef{Owner: "octocat", Name: "tui-old"},
			DefaultBranch: "master",
			Private:       true,
			Fork:          true,
			Archived:      true,
			Template:      true,
			Mirror:        true,
			UpdatedAt:     issueTime("2024-01-26T19:14:43Z"),
			URL:           "https://github.com/octocat/tui-old",
		},
	}
	if len(page.Items) != len(want) {
		t.Fatalf("got %d repos, want %d", len(page.Items), len(want))
	}
	for i := range want {
		if got := page.Items[i]; got != want[i] {
			t.Errorf("repo %d = %+v, want %+v", i, got, want[i])
		}
	}
	if !page.Last() {
		t.Errorf("Next = %q, want the last page without a Link header", page.Next)
	}
}

func TestSearchIssues(t *testing.T) {
	c := serveIssueFixture(t, "search_issues.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/search/issues", map[string]string{"q": "crash"})
	})
	page, err := c.SearchIssues(t.Context(), "crash", "", 0)
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("got %d hits, want 3", len(page.Items))
	}

	issue := page.Items[0]
	wantIssue := wantIssue42
	wantIssue.Assignees = nil
	if issue.Kind != core.SearchIssues || !equalIssue(issue.Issue, wantIssue) {
		t.Errorf("hit 0 = %+v, want issue %+v", issue, wantIssue)
	}

	gh := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	tests := []struct {
		hit    core.SearchHit
		number int
		state  core.State
	}{
		{page.Items[1], 7, core.StateMerged},
		{page.Items[2], 8, core.StateClosed},
	}
	for _, tt := range tests {
		h := tt.hit
		if h.Kind != core.SearchPulls || h.Issue.Repo != gh || h.Issue.Number != tt.number || h.Issue.State != tt.state {
			t.Errorf("hit = %s %s#%d %s, want pulls %s#%d %s",
				h.Kind, h.Issue.Repo, h.Issue.Number, h.Issue.State, gh, tt.number, tt.state)
		}
	}
}

func TestSearchPages(t *testing.T) {
	var queries []string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", `<http://`+r.Host+`/search/issues?q=bug&per_page=5&page=2>; rel="next"`)
		}
		_, _ = w.Write([]byte(`{"total_count": 0, "items": []}`))
	}))

	first, err := c.SearchIssues(t.Context(), "bug", "", 5)
	if err != nil || first.Last() {
		t.Fatalf("first page = %+v, %v; want a next cursor", first, err)
	}
	second, err := c.SearchIssues(t.Context(), "bug", first.Next, 5)
	if err != nil || !second.Last() {
		t.Fatalf("second page = %+v, %v; want the last page", second, err)
	}
	if len(queries) != 2 || queries[1] != "q=bug&per_page=5&page=2" {
		t.Errorf("queries = %q, want the second to follow the Link header", queries)
	}
}

func TestSearchRateLimited(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "30")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1790000000")
		w.Header().Set("X-RateLimit-Resource", "search")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message": "API rate limit exceeded"}`))
	}))
	for name, search := range map[string]func() error{
		"repos": func() error {
			_, err := c.SearchRepos(t.Context(), "tui", "", 0)
			return err
		},
		"issues": func() error {
			_, err := c.SearchIssues(t.Context(), "tui", "", 0)
			return err
		},
	} {
		err := search()
		var rl *core.RateLimitError
		if !errors.As(err, &rl) || !errors.Is(err, core.ErrRateLimited) {
			t.Fatalf("%s: error %v is not a rate limit", name, err)
		}
		if !rl.Reset.Equal(time.Unix(1790000000, 0)) {
			t.Errorf("%s: reset = %v, want the header's", name, rl.Reset)
		}
	}
	if got := c.RateLimit(resourceSearch); got.Resource != "search" || got.Limit != 30 {
		t.Errorf("RateLimit = %+v, want the search quota", got)
	}
}

func TestSearchInvalidQuery(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message": "Validation Failed", "errors": [{"message": "The search is longer than 256 characters."}]}`))
	}))
	_, err := c.SearchRepos(t.Context(), "x", "", 0)
	var ghErr *Error
	if !errors.As(err, &ghErr) || ghErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("error = %v, want a 422", err)
	}
	if want := "Validation Failed (The search is longer than 256 characters.)"; ghErr.Message != want {
		t.Errorf("message = %q, want %q", ghErr.Message, want)
	}
}

func TestRepoFromURL(t *testing.T) {
	tests := map[string]core.RepoRef{
		"https://api.github.com/repos/octo-org/hello":       {Owner: "octo-org", Name: "hello"},
		"https://ghe.example.com/api/v3/repos/team/service": {Owner: "team", Name: "service"},
		"https://api.github.com/":                           {},
		"://bad":                                            {},
	}
	for in, want := range tests {
		if got := repoFromURL(in); got != want {
			t.Errorf("repoFromURL(%q) = %v, want %v", in, got, want)
		}
	}
}
