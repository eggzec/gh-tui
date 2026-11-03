package github

import (
	"net/http"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestPinnedIssues(t *testing.T) {
	c, reqs := serveFixture(t, "graphql_pinned_issues.json")

	got, err := c.PinnedIssues(t.Context(), refsHere)
	if err != nil {
		t.Fatalf("PinnedIssues: %v", err)
	}
	req := <-reqs
	if req.Query != pinnedIssuesQuery {
		t.Errorf("query = %q, want pinnedIssuesQuery", req.Query)
	}
	if v := req.Variables; len(v) != 2 || v["owner"] != refsHere.Owner || v["name"] != refsHere.Name {
		t.Errorf("variables = %v, want the owner and name", v)
	}
	if len(got) != 2 {
		t.Fatalf("PinnedIssues = %d issues, want 2: the null one is dropped", len(got))
	}

	first := got[0]
	if first.ID != "I_10" || first.Number != 10 || first.Repo != refsHere || first.State != core.StateOpen || first.Reason != "" {
		t.Errorf("first = %+v, want the open issue #10", first)
	}
	if first.Author.Login != "octocat" || first.Author.Bot || first.Comments != 3 || first.Body != "Welcome." {
		t.Errorf("first author, comments and body = %+v, %d, %q", first.Author, first.Comments, first.Body)
	}
	if !first.CreatedAt.Equal(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)) || !first.UpdatedAt.Equal(time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)) {
		t.Errorf("first dates = %v, %v", first.CreatedAt, first.UpdatedAt)
	}
	if len(first.Labels) != 1 || first.Labels[0].Name != "docs" || first.Labels[0].Color != "0075ca" {
		t.Errorf("first Labels = %+v, want docs", first.Labels)
	}
	if len(first.Assignees) != 1 || first.Assignees[0].Login != "alice" {
		t.Errorf("first Assignees = %+v, want alice", first.Assignees)
	}
	if !first.Locked || first.LockReason != "too_heated" || first.URL != "https://github.com/eggzec/gh-tui/issues/10" {
		t.Errorf("first lock and URL = %v, %q, %q", first.Locked, first.LockReason, first.URL)
	}

	second := got[1]
	if second.Number != 11 || second.State != core.StateClosed || second.Reason != core.ReasonCompleted || second.Author.Login != "" {
		t.Errorf("second = %+v, want #11 closed as completed, by nobody", second)
	}
}

func TestPinnedIssuesEmpty(t *testing.T) {
	c, _ := serveFixture(t, "graphql_pinned_issues_empty.json")

	got, err := c.PinnedIssues(t.Context(), refsHere)
	if err != nil {
		t.Fatalf("PinnedIssues: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("PinnedIssues = %#v, want an empty slice", got)
	}
}

func TestPinnedIssuesNoRepository(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data": {"repository": null}}`))
	}))

	got, err := c.PinnedIssues(t.Context(), refsHere)
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("PinnedIssues = %#v, %v, want an empty slice and no error", got, err)
	}
}

func TestPinnedIssuesError(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errors": [{"message": "boom"}]}`))
	}))

	if got, err := c.PinnedIssues(t.Context(), refsHere); err == nil {
		t.Errorf("PinnedIssues = %v, want an error", got)
	}
}
