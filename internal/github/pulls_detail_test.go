package github

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestGetPullRequestDetail(t *testing.T) {
	c, reqs := pullServer(t, "pulls_detail_v4.json")
	got, err := c.GetPullRequest(t.Context(), pullsRepo, 42, testSizes)
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	if q := reqs()[0].Query; strings.Contains(q, "viewerViewedState") {
		t.Errorf("detail query selects viewerViewedState, which viewed marks keep locally:\n%s", q)
	}

	t.Run("head and base", func(t *testing.T) {
		if got.BaseSHA != strings.Repeat("2", 40) || got.HeadSHA != strings.Repeat("1", 40) {
			t.Errorf("base %q, head %q; want the commits of the fixture", got.BaseSHA, got.HeadSHA)
		}
		if got.HeadRepo != "octocat/gh-tui" || !got.CrossRepo || got.Milestone != "v0.4" {
			t.Errorf("head repo %q, cross %v, milestone %q; want octocat/gh-tui, true, v0.4", got.HeadRepo, got.CrossRepo, got.Milestone)
		}
	})

	t.Run("merge", func(t *testing.T) {
		want := core.MergeInfo{
			Mergeable:           core.MergeableYes,
			Status:              core.MergeBlocked,
			Methods:             []core.MergeMethod{core.MergeCommit, core.MergeSquash},
			AutoMerge:           &core.AutoMerge{Method: core.MergeSquash, EnabledBy: "monalisa", EnabledAt: pullTime("2026-09-23T09:00:00Z")},
			CanDisableAutoMerge: true,
			RepoAutoMerge:       true,
			CanMergeAsAdmin:     true,
			Queue:               core.MergeQueue{Enabled: true, Queued: true, Position: 2, State: "awaiting_checks", EnqueuedAt: pullTime("2026-09-23T10:00:00Z")},
			Rules: core.MergeRules{
				// The branch protection requires two approvals, a code
				// owner and two checks; a ruleset adds a check, resolved
				// conversations and a linear history, and asks for one
				// approval, which the protection's two cover.
				Known: true, Approvals: 2, Checks: []string{"test (ubuntu-latest)", "codecov/patch", "lint"},
				CodeOwners: true, Conversations: true, LinearHistory: true,
			},
			// Of the required checks: the test run failed, e2e is running
			// and the coverage status failed.
			RequiredChecks: core.CheckCounts{Failed: 2, Pending: 1},
		}
		if !reflect.DeepEqual(got.Merge, want) {
			t.Errorf("merge = %+v, want %+v", got.Merge, want)
		}
		if !got.Merge.Allows(core.MergeSquash) || got.Merge.Allows(core.MergeRebase) {
			t.Errorf("allows squash %v, rebase %v; want true, false", got.Merge.Allows(core.MergeSquash), got.Merge.Allows(core.MergeRebase))
		}
	})

	t.Run("reviewers", func(t *testing.T) {
		want := core.Reviewers{
			Requested: []core.ReviewRequest{
				{User: core.User{Login: "hubot", Name: "Hubot"}, CodeOwner: true},
				{Team: "eggzec/core"},
				// A reviewer the token may not see, such as a private team.
				{Unknown: true},
			},
			RequestedTotal: 4,
			Verdicts: []core.Verdict{
				{Author: core.User{Login: "hubot", Name: "Hubot"}, State: core.ReviewStateChangesRequested, SubmittedAt: pullTime("2026-09-22T12:00:00Z")},
				{Author: core.User{Login: "monalisa"}, State: core.ReviewStateApproved, SubmittedAt: pullTime("2026-09-22T15:30:00Z")},
				{Author: core.User{Login: "copilot", Bot: true}, State: core.ReviewStateDismissed, SubmittedAt: pullTime("2026-09-22T16:00:00Z")},
			},
			VerdictsTotal: 3,
			Viewer:        core.ReviewStatePending,
		}
		if !reflect.DeepEqual(got.Reviewers, want) {
			t.Errorf("reviewers = %+v, want %+v", got.Reviewers, want)
		}
	})

	t.Run("threads", func(t *testing.T) {
		th := got.Threads
		// Five threads, of which the first page of three is listed.
		if th.Total != 5 || !th.Truncated || th.Unresolved != 2 || th.Resolved != 1 || th.Outdated != 1 || len(th.Threads) != 3 {
			t.Errorf("total %d, truncated %v, unresolved %d, resolved %d, outdated %d, listed %d; want 5, true, 2, 1, 1, 3",
				th.Total, th.Truncated, th.Unresolved, th.Resolved, th.Outdated, len(th.Threads))
		}
		first := th.Threads[0]
		want := core.ReviewThread{
			ID: "PRRT_1", Path: "internal/tui/tea_test.go", Line: 54, Comments: 3,
			Author: core.User{Login: "hubot", Name: "Hubot"}, At: pullTime("2026-09-22T12:00:00Z"),
			// One line, without the escape sequence.
			Excerpt: "want a frame, got nothing red",
		}
		if !reflect.DeepEqual(first, want) {
			t.Errorf("first thread = %+v, want %+v", first, want)
		}
		if r := th.Threads[1]; !r.Resolved || r.Author != (core.User{}) {
			t.Errorf("second thread = %+v, want resolved, without an author", r)
		}
		out := th.Threads[2]
		if !out.Outdated || out.Line != 20 || !out.Author.Bot {
			t.Errorf("third thread = %+v, want outdated, at its original line 20, by a bot", out)
		}
		if n := utf8.RuneCountInString(out.Excerpt); n != pullExcerpt || !strings.HasSuffix(out.Excerpt, "…") {
			t.Errorf("long excerpt has %d characters ending %q, want %d ending in an ellipsis", n, out.Excerpt[max(0, len(out.Excerpt)-4):], pullExcerpt)
		}
	})

	t.Run("failing checks", func(t *testing.T) {
		want := []core.FailingCheck{
			{Name: "test (ubuntu-latest)", Run: true, ID: 102, Required: true, Reason: "tea_test.go:54: want a frame, got nothing"},
			{Name: "lint", Run: true, ID: 103, Reason: "Timed out after 10m"},
			{Name: "codecov/patch", Required: true, Reason: "62.50% of diff hit (target 80%)"},
		}
		if !reflect.DeepEqual(got.FailingChecks, want) {
			t.Errorf("failing checks = %+v, want %+v", got.FailingChecks, want)
		}
		// 150 checks, of which 5 are read.
		if !got.ChecksTruncated {
			t.Error("checks not truncated, want truncated: the rollup has 150 and 5 were read")
		}
		if want := (core.CheckCounts{Passed: 146, Failed: 3, Pending: 1}); got.CheckCounts != want {
			t.Errorf("check counts = %+v, want %+v", got.CheckCounts, want)
		}
	})
}

// TestGetPullRequestDetailAbsent reads an answer without any field of the
// detail beyond those of the pull request, as a server that lacks them
// leaves them out: they read as unsaid.
func TestGetPullRequestDetailAbsent(t *testing.T) {
	c, _ := pullServer(t, "pulls_detail.json")
	got, err := c.GetPullRequest(t.Context(), pullsRepo, 42, testSizes)
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	if got.Merge.Rules.Known || got.Merge.Status != "" || got.Merge.AutoMerge != nil || got.Merge.Queue != (core.MergeQueue{}) || len(got.Merge.Methods) != 0 {
		t.Errorf("merge = %+v, want nothing said", got.Merge)
	}
	if got.Reviewers.Requested != nil || got.Reviewers.Viewer != "" || got.Threads.Total != 0 || got.Threads.Truncated || got.FailingChecks != nil || got.HeadRepo != "" {
		t.Errorf("reviewers %+v, threads %+v, failing %v; want none", got.Reviewers, got.Threads, got.FailingChecks)
	}
}

// TestGetPullRequestDetailDenied keeps the detail when the token may not
// read the branch rules or the merge queue, and fails when it may not read
// anything else.
func TestGetPullRequestDetailDenied(t *testing.T) {
	c, _ := pullServer(t, "pulls_detail_denied.json")
	got, err := c.GetPullRequest(t.Context(), pullsRepo, 42, testSizes)
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	// The team the token may not see reads as unknown, beside the person
	// it may.
	wantReq := []core.ReviewRequest{{User: core.User{Login: "hubot", Name: "Hubot"}}, {Unknown: true, CodeOwner: true}}
	if !reflect.DeepEqual(got.Reviewers.Requested, wantReq) {
		t.Errorf("requested = %+v, want %+v", got.Reviewers.Requested, wantReq)
	}
	if got.Number != 42 || got.Merge.Rules.Known || got.Merge.Mergeable != core.MergeableUnknown {
		t.Errorf("detail = number %d, rules known %v, mergeable %q; want 42, false, unknown", got.Number, got.Merge.Rules.Known, got.Merge.Mergeable)
	}
	if want := []core.MergeMethod{core.MergeCommit}; !reflect.DeepEqual(got.Merge.Methods, want) {
		t.Errorf("methods = %v, want %v", got.Merge.Methods, want)
	}

	c, _ = pullServer(t, "pulls_detail_denied_body.json")
	if _, err := c.GetPullRequest(t.Context(), pullsRepo, 42, testSizes); err == nil {
		t.Error("GetPullRequest succeeded though the token may not read the pull request's body")
	}
}

// testSizes are the sizes of the lists the tests read the detail with.
var testSizes = DetailSizes{Threads: 50, Reviewers: 10, Rules: 20}

func TestGetPullRequestDetailSizes(t *testing.T) {
	for _, tt := range []struct{ asked, sent DetailSizes }{
		{testSizes, testSizes},
		{DetailSizes{}, DetailSizes{1, 1, 1}},
		{DetailSizes{500, 101, 100}, DetailSizes{100, 100, 100}},
	} {
		c, reqs := pullServer(t, "pulls_detail.json")
		if _, err := c.GetPullRequest(t.Context(), pullsRepo, 42, tt.asked); err != nil {
			t.Fatalf("GetPullRequest: %v", err)
		}
		v := reqs()[0].Variables
		got := DetailSizes{int(v["threads"].(float64)), int(v["reviewers"].(float64)), int(v["rules"].(float64))}
		if got != tt.sent {
			t.Errorf("asked %+v, sent %+v, want %+v", tt.asked, got, tt.sent)
		}
	}
}

func TestExcerpt(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"  one\n two\t three ", "one two three"},
		{"", ""},
		{strings.Repeat("é", pullExcerpt), strings.Repeat("é", pullExcerpt)},
		{strings.Repeat("é", pullExcerpt+1), strings.Repeat("é", pullExcerpt-1) + "…"},
	} {
		if got := excerpt(tt.in); got != tt.want {
			t.Errorf("excerpt(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestRefusalIsLoggedOncePerRepository checks that a field refused again
// on every read is told of once per repository, whatever the index of the
// node in its path.
func TestRefusalIsLoggedOncePerRepository(t *testing.T) {
	c := newTestClient(t, nil)
	ctx := withCall(t.Context(), &call{repo: "eggzec/gh-tui"})
	refusal := func(i int) GraphQLErrorItem {
		return GraphQLErrorItem{Type: "FORBIDDEN", Path: []any{"repository", "pullRequest", "reviewRequests", "nodes", float64(i), "requestedReviewer"}}
	}
	if !c.firstRefusal(ctx, refusal(0)) {
		t.Error("first refusal not first")
	}
	if c.firstRefusal(ctx, refusal(3)) {
		t.Error("the same field of another node is first again")
	}
	if !c.firstRefusal(withCall(t.Context(), &call{repo: "eggzec/other"}), refusal(0)) {
		t.Error("the same field of another repository isn't first")
	}
	other := refusal(0)
	other.Path = []any{"repository", "pullRequest", "baseRef"}
	if !c.firstRefusal(ctx, other) {
		t.Error("another field isn't first")
	}
}
