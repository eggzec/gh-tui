package github

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestPullLinks(t *testing.T) {
	c, reqs := serveFixture(t, "graphql_pull_links_basic.json")

	got, err := c.PullLinks(t.Context(), refsHere, 7)
	if err != nil {
		t.Fatalf("PullLinks: %v", err)
	}
	req := <-reqs
	if req.Query != pullLinksQuery {
		t.Errorf("query = %q, want pullLinksQuery", req.Query)
	}
	if v := req.Variables; len(v) != 3 || v["number"] != 7.0 {
		t.Errorf("variables = %v, want the owner, name and 7", v)
	}
	// Blockers are read one level deep: no selection of blockedBy holds another.
	if n := strings.Count(req.Query, "blockedBy("); n != 1 {
		t.Errorf("query selects blockedBy %d times, want once", n)
	}

	if got.ClosesTotal != 30 || len(got.Closes) != 2 || !got.BlockersSupported {
		t.Fatalf("PullLinks = %d of %d closed, supported %v; want 2 of 30, supported", len(got.Closes), got.ClosesTotal, got.BlockersSupported)
	}
	// Two issues are closed by #20, and the pull request closes both: one
	// entry for #20, and none for the pull request itself.
	linked := make([]int, 0, len(got.Linked))
	for _, l := range got.Linked {
		linked = append(linked, l.Target.Number)
		if l.Target.Kind != core.KindPull {
			t.Errorf("linked %+v has kind %q, want a pull request", l.Target, l.Target.Kind)
		}
	}
	if len(linked) != 2 || linked[0] != 20 || linked[1] != 21 {
		t.Errorf("Linked numbers = %v, want [20 21]", linked)
	}
	if got.Linked[0].State != core.StateMerged || got.Linked[1].State != core.StateOpen || !got.Linked[1].Draft {
		t.Errorf("Linked = %+v, want #20 merged and #21 an open draft", got.Linked)
	}
	if !got.LinkedCapped {
		t.Error("LinkedCapped = false, want true: the first issue has 4 pull requests and 3 were read")
	}

	first, second := got.Closes[0], got.Closes[1]
	if first.Issue.Target.Number != 10 || first.Issue.State != core.StateOpen || first.Issue.Target.Kind != core.KindIssue {
		t.Errorf("first closed = %+v, want the open issue #10", first.Issue)
	}
	if first.BlockersTotal != 12 || len(first.Blockers) != 1 || first.Blockers[0].Target.Number != 5 ||
		first.Blockers[0].State != core.StateClosed || first.Blockers[0].Reason != core.ReasonCompleted {
		t.Errorf("first blockers = %d of %d: %+v, want #5 of 12", len(first.Blockers), first.BlockersTotal, first.Blockers)
	}
	if second.Issue.State != core.StateClosed || second.Issue.Reason != core.ReasonNotPlanned ||
		second.Issue.Target.Repo != (core.RepoRef{Owner: "acme", Name: "lib"}) {
		t.Errorf("second closed = %+v, want #11 of acme/lib, closed as not planned", second.Issue)
	}
	if len(second.Blockers) != 0 || second.BlockersTotal != 0 {
		t.Errorf("second blockers = %+v of %d, want none", second.Blockers, second.BlockersTotal)
	}
}

// A node that GitHub refuses leaves the rest of the read, as a row that says
// why.
func TestPullLinksForbidden(t *testing.T) {
	c, _ := serveFixture(t, "graphql_pull_links_forbidden.json")

	got, err := c.PullLinks(t.Context(), refsHere, 7)
	if err != nil {
		t.Fatalf("PullLinks: %v", err)
	}
	if len(got.Closes) != 2 || got.Closes[0].Issue.Problem == "" || got.Closes[0].Issue.Target.Number != 0 {
		t.Fatalf("Closes = %+v, want a row with a problem first", got.Closes)
	}
	if r := got.Closes[1].Issue; r.Problem != "" || r.Target.Number != 10 {
		t.Errorf("second closed = %+v, want #10 read", r)
	}
	if got.LinkedCapped || len(got.Linked) != 0 {
		t.Errorf("Linked = %+v, capped %v; want none, not capped", got.Linked, got.LinkedCapped)
	}
}

// The pull requests of this repository come first, then those of other
// repositories, each by number, and a pull request GitHub refuses last, as a
// row that says why.
func TestPullLinksLinkedOrder(t *testing.T) {
	c, _ := serveFixture(t, "graphql_pull_links_order.json")

	got, err := c.PullLinks(t.Context(), refsHere, 7)
	if err != nil {
		t.Fatalf("PullLinks: %v", err)
	}
	var order []string
	for _, l := range got.Linked {
		if l.Problem != "" {
			order = append(order, "problem")
			continue
		}
		order = append(order, l.Target.Repo.String()+"#"+strconv.Itoa(l.Target.Number))
	}
	want := []string{"eggzec/gh-tui#9", "eggzec/gh-tui#20", "acme/lib#3", "acme/lib#8", "problem"}
	if !slices.Equal(order, want) {
		t.Errorf("Linked = %v, want %v", order, want)
	}
}

func TestPullLinksNotFound(t *testing.T) {
	c, _ := serveFixture(t, "graphql_pull_preview_missing.json")
	if _, err := c.PullLinks(t.Context(), refsHere, 7); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("PullLinks err = %v, want core.ErrNotFound", err)
	}
}

// A server without issue dependencies answers without blockers, once: both
// reads succeed with BlockersSupported false.
func TestLinkedWithoutBlockers(t *testing.T) {
	read := func(t *testing.T, fixture string) (*Client, *enterpriseServer) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("testdata", fixture))
		if err != nil {
			t.Fatal(err)
		}
		srv := &enterpriseServer{t: t, data: data, lacks: map[string]string{"blockedBy": "Issue"}}
		return newEnterpriseClient(t, srv), srv
	}

	t.Run("pull links", func(t *testing.T) {
		c, srv := read(t, "graphql_pull_links_ghes.json")
		for i := range 2 {
			got, err := c.PullLinks(t.Context(), refsHere, 7)
			if err != nil {
				t.Fatalf("PullLinks %d: %v", i, err)
			}
			if got.BlockersSupported || len(got.Closes) != 1 || len(got.Closes[0].Blockers) != 0 || got.Closes[0].BlockersTotal != 0 {
				t.Errorf("PullLinks %d = %+v, want the issue without blockers, unsupported", i, got)
			}
		}
		sent := srv.sent()
		if len(sent) != 3 {
			t.Fatalf("sent %d queries, want the first, it again without blockedBy, and the second read without it", len(sent))
		}
		if strings.Contains(sent[1], "blockedBy") || !strings.Contains(sent[1], "closedByPullRequestsReferences") {
			t.Errorf("query = %q, want pullLinksQuery without blockedBy only", sent[1])
		}
	})

	t.Run("issue preview", func(t *testing.T) {
		c, _ := read(t, "graphql_issue_preview_ghes.json")
		got, err := c.IssuePreview(t.Context(), refsHere, 10)
		if err != nil {
			t.Fatalf("IssuePreview: %v", err)
		}
		if got.BlockersSupported || len(got.Blockers) != 0 || got.Title != "Token refresh fails" {
			t.Errorf("IssuePreview = %+v, want the issue without blockers, unsupported", got)
		}
	})

	t.Run("github.com", func(t *testing.T) {
		c, _ := serveFixture(t, "graphql_issue_preview.json")
		got, err := c.IssuePreview(t.Context(), refsHere, 10)
		if err != nil {
			t.Fatalf("IssuePreview: %v", err)
		}
		if !got.BlockersSupported || len(got.Blockers) != 1 {
			t.Errorf("IssuePreview = %+v, want its blockers, supported", got)
		}
	})
}

func TestIssuePreview(t *testing.T) {
	c, reqs := serveFixture(t, "graphql_issue_preview.json")

	got, err := c.IssuePreview(t.Context(), refsHere, 10)
	if err != nil {
		t.Fatalf("IssuePreview: %v", err)
	}
	if req := <-reqs; req.Query != issuePreviewQuery {
		t.Errorf("query = %q, want issuePreviewQuery", req.Query)
	}
	if got.Target.Number != 10 || got.Target.Kind != core.KindIssue || got.State != core.StateClosed || got.Reason != core.ReasonNotPlanned {
		t.Errorf("IssuePreview = %+v, want #10, closed as not planned", got.Reference)
	}
	if got.Author != "octocat" || !got.CreatedAt.Equal(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)) || got.Body != "The token is not refreshed." {
		t.Errorf("author, date and body = %q, %v, %q", got.Author, got.CreatedAt, got.Body)
	}
	if len(got.Labels) != 1 || got.Labels[0].Name != "bug" || got.Labels[0].Color != "d73a4a" {
		t.Errorf("Labels = %+v, want bug", got.Labels)
	}
	if got.BlockersTotal != 4 || len(got.Blockers) != 1 || got.Blockers[0].Target.Number != 5 {
		t.Errorf("blockers = %+v of %d, want #5 of 4", got.Blockers, got.BlockersTotal)
	}
	if got.CommentsTotal != 9 || len(got.Comments) != 2 || got.Comments[0].Author != "alice" || got.Comments[1].Author != "" {
		t.Errorf("comments = %+v of %d, want 2 of 9, the second from a deleted account", got.Comments, got.CommentsTotal)
	}
}

// A blocker that GitHub refuses leaves the rest of the preview, as a row that
// says why.
func TestIssuePreviewForbiddenBlocker(t *testing.T) {
	c, _ := serveFixture(t, "graphql_issue_preview_forbidden.json")

	got, err := c.IssuePreview(t.Context(), refsHere, 10)
	if err != nil {
		t.Fatalf("IssuePreview: %v", err)
	}
	if got.Target.Number != 10 || got.Body != "The token is not refreshed." || len(got.Comments) != 1 {
		t.Errorf("IssuePreview = %+v, want #10 with its body and comment", got)
	}
	if len(got.Blockers) != 1 || got.Blockers[0].Problem == "" || got.BlockersTotal != 1 {
		t.Errorf("blockers = %+v of %d, want one row with a problem", got.Blockers, got.BlockersTotal)
	}
}

func TestPullPreview(t *testing.T) {
	c, reqs := serveFixture(t, "graphql_pull_preview.json")

	got, err := c.PullPreview(t.Context(), refsHere, 20)
	if err != nil {
		t.Fatalf("PullPreview: %v", err)
	}
	if req := <-reqs; req.Query != pullPreviewQuery {
		t.Errorf("query = %q, want pullPreviewQuery", req.Query)
	}
	if got.Target.Number != 20 || got.Target.Kind != core.KindPull || got.State != core.StateMerged {
		t.Errorf("PullPreview = %+v, want #20 merged", got.Reference)
	}
	if got.Checks != (core.CheckCounts{Passed: 3, Pending: 1}) {
		t.Errorf("Checks = %+v, want 3 passed and 1 pending", got.Checks)
	}
	if got.Body != "Refreshes the token\nbefore it expires." {
		t.Errorf("Body = %q, want it kept with its line breaks", got.Body)
	}
	if got.Head != "fix/refresh" || got.Base != "main" || got.Additions != 40 || got.Deletions != 3 || got.ChangedFiles != 5 ||
		got.ReviewDecision != core.ReviewChangesRequested || got.Author != "octocat" {
		t.Errorf("PullPreview = %+v", got)
	}
	if len(got.Reviews) != 2 || got.Reviews[0] != (core.PreviewReview{Author: "alice", State: core.ReviewStateApproved}) ||
		got.Reviews[1].State != core.ReviewStateChangesRequested {
		t.Errorf("Reviews = %+v, want alice's approval and bob's request", got.Reviews)
	}
}

func TestPullPreviewNotFound(t *testing.T) {
	c, _ := serveFixture(t, "graphql_pull_preview_missing.json")
	if _, err := c.PullPreview(t.Context(), refsHere, 20); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("PullPreview err = %v, want core.ErrNotFound", err)
	}
	c, _ = serveFixture(t, "graphql_pull_preview_missing.json")
	if _, err := c.IssuePreview(t.Context(), refsHere, 20); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("IssuePreview err = %v, want core.ErrNotFound", err)
	}
}
