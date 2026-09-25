package github

import (
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestViewerWork(t *testing.T) {
	c, reqs := serveFixture(t, "viewer_work.json")

	got, err := c.ViewerWork(t.Context(), 2)
	if err != nil {
		t.Fatalf("ViewerWork: %v", err)
	}

	req := <-reqs
	if req.Query != viewerWorkQuery {
		t.Errorf("query = %q, want viewerWorkQuery", req.Query)
	}
	if v := req.Variables; len(v) != 1 || v["first"] != 2.0 {
		t.Errorf("variables = %v, want first 2", v)
	}
	counts := [3]int{got.ReviewRequested.Count, got.Authored.Count, got.Assigned.Count}
	if counts != [3]int{117, 23, 133} {
		t.Errorf("counts = %v, want [117 23 133]", counts)
	}
	for name, l := range map[string]core.WorkList{"review requested": got.ReviewRequested, "authored": got.Authored, "assigned": got.Assigned} {
		if len(l.Items) != 2 {
			t.Errorf("%s: %d items, want 2", name, len(l.Items))
		}
	}

	pr := core.SearchHit{Kind: core.SearchPulls, Issue: core.Issue{
		ID:        "PR_kwDOOt6mSM8AAAABErRX6g",
		Repo:      core.RepoRef{Owner: "charmbracelet", Name: "crush"},
		Number:    3926,
		Title:     "ui: add mouse support to top-level command palette",
		State:     core.StateOpen,
		Author:    core.User{Login: "meowgorithm"},
		CreatedAt: time.Date(2026, 9, 23, 1, 49, 27, 0, time.UTC),
		UpdatedAt: time.Date(2026, 9, 24, 7, 10, 46, 0, time.UTC),
		URL:       "https://github.com/charmbracelet/crush/pull/3926",
	}}
	if h := got.Authored.Items[0]; h.Kind != pr.Kind || h.Issue.ID != pr.Issue.ID || h.Issue.Repo != pr.Issue.Repo ||
		h.Issue.Number != pr.Issue.Number || h.Issue.Title != pr.Issue.Title || h.Issue.State != pr.Issue.State ||
		h.Issue.Author != pr.Issue.Author || !h.Issue.CreatedAt.Equal(pr.Issue.CreatedAt) ||
		!h.Issue.UpdatedAt.Equal(pr.Issue.UpdatedAt) || h.Issue.URL != pr.Issue.URL {
		t.Errorf("authored[0] = %+v\nwant %+v", h, pr)
	}
	if !got.Authored.Items[0].Draft || got.ReviewRequested.Items[0].Draft {
		t.Errorf("drafts = %v, %v; want the authored pull request only", got.Authored.Items[0].Draft, got.ReviewRequested.Items[0].Draft)
	}

	issue := got.Assigned.Items[1]
	if issue.Kind != core.SearchIssues || issue.Issue.Number != 1053 || issue.Issue.Repo != (core.RepoRef{Owner: "charmbracelet", Name: "bubbles"}) {
		t.Errorf("assigned[1] = %+v, want issue charmbracelet/bubbles#1053", issue)
	}
	// A deleted account leaves no author.
	if issue.Issue.Author != (core.User{}) {
		t.Errorf("assigned[1] author = %+v, want none", issue.Issue.Author)
	}
}

func TestViewerWorkEmpty(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"reviewRequested":{"issueCount":0,"nodes":[]},"authored":{"issueCount":0,"nodes":[]},"assigned":{"issueCount":0,"nodes":[]}}}`)
	}))

	got, err := c.ViewerWork(t.Context(), 10)
	if err != nil || got.ReviewRequested.Items != nil || got.Authored.Count != 0 || got.Assigned.Items != nil {
		t.Errorf("ViewerWork = %+v, %v; want nothing waiting", got, err)
	}
}

func TestViewerWorkRateLimited(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1790000000")
		_, _ = io.WriteString(w, `{"data":null,"errors":[{"type":"RATE_LIMITED","message":"API rate limit exceeded"}]}`)
	}))

	_, err := c.ViewerWork(t.Context(), 10)
	if _, ok := errors.AsType[*core.RateLimitError](err); !ok {
		t.Errorf("error = %v, want a *core.RateLimitError", err)
	}
}
