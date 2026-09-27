package github

import (
	"bytes"
	"encoding/json"
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

func TestSearch(t *testing.T) {
	c, reqs := serveFixture(t, "search_multi.json")
	got, err := c.Search(t.Context(), SearchQuery{Text: "lipgloss"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	req := <-reqs
	if op := operation(req.Query); op != "Search" {
		t.Errorf("operation = %q, want Search", op)
	}
	wantVars := map[string]any{
		"reposQuery": "lipgloss", "issuesQuery": "lipgloss is:issue", "pullsQuery": "lipgloss is:pr",
		"reposFirst": 20.0, "issuesFirst": 20.0, "pullsFirst": 20.0, "withRepos": true, "withIssues": true, "withPulls": true,
	}
	checkSearchVars(t, req.Variables, wantVars)

	totals := map[core.SearchKind]int{core.SearchRepos: 499, core.SearchIssues: 4478, core.SearchPulls: 26860}
	if len(got) != len(totals) {
		t.Fatalf("got kinds %v, want %v", slices.Collect(maps.Keys(got)), slices.Collect(maps.Keys(totals)))
	}
	for kind, total := range totals {
		p := got[kind]
		if p.Total != total || len(p.Items) != 20 || p.Next != "Y3Vyc29yOjIw" {
			t.Errorf("%s: total %d, %d hits, next %q; want %d, 20, a next cursor", kind, p.Total, len(p.Items), p.Next, total)
		}
		for i, h := range p.Items {
			if h.Kind != kind {
				t.Errorf("%s hit %d has kind %s", kind, i, h.Kind)
			}
		}
	}

	repo := got[core.SearchRepos].Items[0].Repo
	wantRepo := core.Repo{
		ID:            "MDEwOlJlcG9zaXRvcnkzNDM1MzM0MTk=",
		Ref:           core.RepoRef{Owner: "charmbracelet", Name: "lipgloss"},
		Description:   "Style definitions for nice terminal layouts 👄",
		DefaultBranch: "main",
		Language:      "Go",
		LanguageColor: "#00ADD8",
		Stars:         repo.Stars,
		UpdatedAt:     repo.UpdatedAt,
		URL:           "https://github.com/charmbracelet/lipgloss",
	}
	if repo != wantRepo || repo.Stars < 10000 || repo.UpdatedAt.IsZero() {
		t.Errorf("repo 0 = %+v, want %+v", repo, wantRepo)
	}

	issue := got[core.SearchIssues].Items[0].Issue
	if issue.Repo.String() != "EasternEdgeRobotics/illusion" || issue.Number != 29 || issue.State != core.StateOpen ||
		issue.Author != (core.User{Login: "Peyton-C", Name: "Peyton Cashin"}) || issue.Comments != 1 ||
		len(issue.Labels) != 1 || issue.Labels[0].Name != "lipgloss" ||
		issue.URL != "https://github.com/EasternEdgeRobotics/illusion/issues/29" {
		t.Errorf("issue 0 = %+v", issue)
	}
}

func TestSearchStates(t *testing.T) {
	c, reqs := serveFixture(t, "search_multi_closed.json")
	text := "repo:charmbracelet/lipgloss is:closed table"
	got, err := c.Search(t.Context(), SearchQuery{Text: text, First: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// The user's qualifiers go to every kind, and each kind adds its own.
	checkSearchVars(t, (<-reqs).Variables, map[string]any{
		"reposQuery": text, "issuesQuery": text + " is:issue", "pullsQuery": text + " is:pr", "reposFirst": 5.0, "pullsFirst": 5.0,
	})

	if p := got[core.SearchRepos]; p.Total != 0 || len(p.Items) != 0 || !p.Last() {
		t.Errorf("repos = %+v, want none", p)
	}
	for kind, want := range map[core.SearchKind][]core.State{
		core.SearchIssues: {core.StateClosed, core.StateClosed, core.StateClosed, core.StateClosed, core.StateClosed},
		core.SearchPulls:  {core.StateMerged, core.StateClosed, core.StateMerged, core.StateClosed, core.StateMerged},
	} {
		p := got[kind]
		if len(p.Items) != len(want) {
			t.Fatalf("%s: %d hits, want %d", kind, len(p.Items), len(want))
		}
		for i, h := range p.Items {
			if h.Issue.State != want[i] || h.Issue.Repo.String() != "charmbracelet/lipgloss" {
				t.Errorf("%s hit %d = %s %s, want %s in charmbracelet/lipgloss", kind, i, h.Issue.Repo, h.Issue.State, want[i])
			}
		}
	}
	reasons := make([]core.StateReason, 0, 5)
	for _, h := range got[core.SearchIssues].Items {
		reasons = append(reasons, h.Issue.Reason)
	}
	if want := []core.StateReason{core.ReasonCompleted, core.ReasonNotPlanned, core.ReasonCompleted, core.ReasonCompleted, core.ReasonCompleted}; !slices.Equal(reasons, want) {
		t.Errorf("reasons = %v, want %v", reasons, want)
	}
	if n := got[core.SearchPulls].Items[0].Issue; n.Number != 697 || n.Author.Login != "pete-woods" || n.Comments != 6 {
		t.Errorf("pull 0 = %+v", n)
	}
}

func TestSearchOneKind(t *testing.T) {
	tests := []struct {
		name, fixture, text string
		after               map[core.SearchKind]string
		wantVars            map[string]any
		wantDrafts          []bool
	}{
		{
			name:    "next page",
			fixture: "search_pulls_page.json",
			text:    "lipgloss",
			after:   map[core.SearchKind]string{core.SearchPulls: "Y3Vyc29yOjIw"},
			wantVars: map[string]any{
				"withRepos": false, "withIssues": false, "withPulls": true, "pullsAfter": "Y3Vyc29yOjIw",
			},
		},
		{
			name:       "first page",
			fixture:    "search_pulls_draft.json",
			text:       "repo:charmbracelet/lipgloss border",
			after:      map[core.SearchKind]string{core.SearchPulls: ""},
			wantVars:   map[string]any{"withRepos": false, "withIssues": false, "withPulls": true},
			wantDrafts: []bool{false, false, false, true, false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := serveFixture(t, tt.fixture)
			got, err := c.Search(t.Context(), SearchQuery{Text: tt.text, After: tt.after})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			vars := (<-reqs).Variables
			checkSearchVars(t, vars, tt.wantVars)
			if vars["pullsQuery"] != tt.text+" is:pr" {
				t.Errorf("pullsQuery = %v, want the text and is:pr", vars["pullsQuery"])
			}
			for _, v := range []string{"reposAfter", "issuesAfter", "pullsAfter"} {
				if _, ok := tt.wantVars[v]; !ok && vars[v] != nil {
					t.Errorf("%s = %v, want it unset", v, vars[v])
				}
			}
			if len(got) != 1 {
				t.Fatalf("got kinds %v, want pulls only", slices.Collect(maps.Keys(got)))
			}
			p := got[core.SearchPulls]
			if p.Total == 0 || len(p.Items) == 0 || p.Last() {
				t.Errorf("pulls = total %d, %d hits, next %q", p.Total, len(p.Items), p.Next)
			}
			for i, want := range tt.wantDrafts {
				if p.Items[i].Draft != want {
					t.Errorf("pull %d draft = %t, want %t", i, p.Items[i].Draft, want)
				}
			}
		})
	}
}

func TestSearchCountsOnly(t *testing.T) {
	var got map[string]any
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		got = req.Variables
		_, _ = w.Write([]byte(`{"data": {
			"repos": {"repositoryCount": 7, "pageInfo": {"hasNextPage": false}, "nodes": []},
			"issues": {"issueCount": 42, "pageInfo": {"hasNextPage": true, "endCursor": "c"}, "nodes": []},
			"pulls": {"issueCount": 3, "pageInfo": {"hasNextPage": false}, "nodes": []}}}`))
	}))
	pages, err := c.Search(t.Context(), SearchQuery{
		Text:  "tui",
		After: map[core.SearchKind]string{core.SearchRepos: ""},
		Count: []core.SearchKind{core.SearchIssues, core.SearchPulls},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	checkSearchVars(t, got, map[string]any{
		"reposFirst": 20.0, "issuesFirst": 0.0, "pullsFirst": 0.0, "withRepos": true, "withIssues": true, "withPulls": true,
	})
	if pages[core.SearchIssues].Total != 42 || pages[core.SearchPulls].Total != 3 || pages[core.SearchRepos].Total != 7 {
		t.Errorf("pages = %+v, want every kind counted", pages)
	}
}

func TestSearchRefusesQuery(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("unexpected request")
	}))
	for name, q := range map[string]SearchQuery{
		"code":           {Text: "x", After: map[core.SearchKind]string{core.SearchCode: ""}},
		"page size":      {Text: "x", First: 101},
		"count code":     {Text: "x", Count: []core.SearchKind{core.SearchCode}},
		"list and count": {Text: "x", After: map[core.SearchKind]string{core.SearchRepos: ""}, Count: []core.SearchKind{core.SearchRepos}},
	} {
		if _, err := c.Search(t.Context(), q); err == nil {
			t.Errorf("%s: Search succeeded, want an error", name)
		}
	}
}

func TestSearchGraphQLRateLimited(t *testing.T) {
	reset := time.Unix(1790000000, 0)
	c := newTestClientAt(t, reset.Add(-time.Minute), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1790000000")
		w.Header().Set("X-RateLimit-Resource", "graphql")
		_, _ = w.Write([]byte(`{"data": null, "errors": [{"type": "RATE_LIMITED", "message": "API rate limit exceeded"}]}`))
	}))
	_, err := c.Search(t.Context(), SearchQuery{Text: "x"})
	rl, ok := errors.AsType[*core.RateLimitError](err)
	if !ok || !rl.Reset.Equal(reset.Add(minGuard)) {
		t.Fatalf("error = %v, want a rate limit until a guard after the header's reset", err)
	}
}

func checkSearchVars(t *testing.T, got, want map[string]any) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("variable %s = %#v, want %#v", k, got[k], v)
		}
	}
}

// BenchmarkDecodeSearch measures decoding a search of every kind with 20
// results each, as the client does it: the GraphQL envelope, then the data.
func BenchmarkDecodeSearch(b *testing.B) {
	body, err := os.ReadFile(filepath.Join("testdata", "search_multi.json"))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := decode(b.Context(), bytes.NewReader(body), &env); err != nil {
			b.Fatal(err)
		}
		_ = queryRate(env.Data)
		var data searchData
		if err := json.Unmarshal(env.Data, &data); err != nil {
			b.Fatal(err)
		}
		if p := data.pages(); len(p[core.SearchPulls].Items) != 20 {
			b.Fatalf("got %d pulls", len(p[core.SearchPulls].Items))
		}
	}
}
