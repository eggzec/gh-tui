package github

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// sequenceServer answers the n-th GraphQL request with answers[n], and
// every one after the last with the last. It returns the client and the
// requests received so far.
func sequenceServer(t *testing.T, answers ...string) (c *Client, requests func() []pullQuery) {
	t.Helper()
	var (
		mu   sync.Mutex
		reqs []pullQuery
	)
	c = newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req pullQuery
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		n := min(len(reqs), len(answers)-1)
		reqs = append(reqs, req)
		mu.Unlock()
		_, _ = w.Write([]byte(answers[n]))
	}))
	return c, func() []pullQuery {
		mu.Lock()
		defer mu.Unlock()
		return append([]pullQuery(nil), reqs...)
	}
}

const (
	reviewAdded     = `{"data":{"result":{"pullRequestReview":{"id":"PRR_pending"}}}}`
	reviewReplied   = `{"data":{"result":{"comment":{"id":"PRRC_reply"}}}}`
	reviewSubmitted = `{"data":{"result":{"pullRequestReview":{"id":"PRR_pending"}}}}`
	reviewDeleted   = `{"data":{"result":{"pullRequestReview":{"id":"PRR_pending"}}}}`
	reviewRefused   = `{"data":{"result":null},"errors":[{"type":"UNPROCESSABLE","message":"Could not submit"}]}`
)

var (
	reviewThreads = []ReviewThreadDraft{
		{Path: "a.go", Side: "RIGHT", Line: 7, Body: "one line"},
		{Path: "b.go", Side: "LEFT", Line: 12, StartLine: 9, Body: "a range"},
	}
	reviewReplies = []ReviewReplyDraft{{ThreadID: "PRRT_1", Body: "a reply"}}
)

func TestSubmitReview(t *testing.T) {
	c, reqs := sequenceServer(t, reviewAdded, reviewReplied, reviewSubmitted)
	id, err := c.SubmitReview(t.Context(), "PR_1", "abc123", core.ReviewEventApprove, "looks good", reviewThreads, reviewReplies)
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if id != "PRR_pending" {
		t.Errorf("review = %q, want PRR_pending", id)
	}
	got := reqs()
	if len(got) != 3 {
		t.Fatalf("got %d requests, want 3 (start, reply, submit)", len(got))
	}
	want := []struct {
		op   string
		vars map[string]any
	}{
		{"addPullRequestReview(", map[string]any{
			"pull": "PR_1", "commit": "abc123",
			"threads": []any{
				map[string]any{"path": "a.go", "side": "RIGHT", "line": float64(7), "body": "one line"},
				map[string]any{"path": "b.go", "side": "LEFT", "line": float64(12), "startLine": float64(9), "startSide": "LEFT", "body": "a range"},
			},
		}},
		{"addPullRequestReviewThreadReply(", map[string]any{"review": "PRR_pending", "thread": "PRRT_1", "body": "a reply"}},
		{"submitPullRequestReview(", map[string]any{"pull": "PR_1", "review": "PRR_pending", "event": "APPROVE", "body": "looks good"}},
	}
	for i, w := range want {
		if !strings.Contains(got[i].Query, w.op) {
			t.Errorf("request %d query does not contain %q:\n%s", i, w.op, got[i].Query)
		}
		if !reflect.DeepEqual(got[i].Variables, w.vars) {
			t.Errorf("request %d variables = %#v, want %#v", i, got[i].Variables, w.vars)
		}
	}
	// The pending review must not name an event, or GitHub submits it at once.
	if strings.Contains(got[0].Query, "event") {
		t.Errorf("the request that starts the review names an event:\n%s", got[0].Query)
	}
}

const reviewFileAdded = `{"data":{"result":{"thread":{"id":"PRRT_file"}}}}`

// A comment on a whole file has no line, so it is not one of the threads that
// start the review: it is added to the pending review, before its submission.
func TestSubmitReviewFileComments(t *testing.T) {
	c, reqs := sequenceServer(t, reviewAdded, reviewFileAdded, reviewReplied, reviewSubmitted)
	threads := []ReviewThreadDraft{
		reviewThreads[0],
		{Path: "c.go", File: true, Body: "about the file"},
	}
	if _, err := c.SubmitReview(t.Context(), "PR_1", "abc123", core.ReviewEventComment, "", threads, reviewReplies); err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	got := reqs()
	want := []struct {
		op   string
		vars map[string]any
	}{
		{"addPullRequestReview(", map[string]any{
			"pull": "PR_1", "commit": "abc123",
			"threads": []any{map[string]any{"path": "a.go", "side": "RIGHT", "line": float64(7), "body": "one line"}},
		}},
		{"subjectType: FILE", map[string]any{"pull": "PR_1", "review": "PRR_pending", "path": "c.go", "body": "about the file"}},
		{"addPullRequestReviewThreadReply(", map[string]any{"review": "PRR_pending", "thread": "PRRT_1", "body": "a reply"}},
		{"submitPullRequestReview(", map[string]any{"pull": "PR_1", "review": "PRR_pending", "event": "COMMENT", "body": ""}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d requests, want %d", len(got), len(want))
	}
	for i, w := range want {
		if !strings.Contains(got[i].Query, w.op) {
			t.Errorf("request %d query does not contain %q:\n%s", i, w.op, got[i].Query)
		}
		if !reflect.DeepEqual(got[i].Variables, w.vars) {
			t.Errorf("request %d variables = %#v, want %#v", i, got[i].Variables, w.vars)
		}
	}
}

func TestSubmitReviewFileCommentsOnly(t *testing.T) {
	c, reqs := sequenceServer(t, reviewAdded, reviewFileAdded, reviewSubmitted)
	threads := []ReviewThreadDraft{{Path: "c.go", File: true, Body: "about the file"}}
	if _, err := c.SubmitReview(t.Context(), "PR_1", "abc123", core.ReviewEventComment, "", threads, nil); err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if _, ok := reqs()[0].Variables["threads"]; ok {
		t.Errorf("start variables = %#v, want no threads", reqs()[0].Variables)
	}
}

func TestSubmitReviewFileCommentFailureDeletesThePendingReview(t *testing.T) {
	c, reqs := sequenceServer(t, reviewAdded, reviewRefused, reviewDeleted)
	threads := []ReviewThreadDraft{{Path: "c.go", File: true, Body: "about the file"}}
	if _, err := c.SubmitReview(t.Context(), "PR_1", "abc123", core.ReviewEventComment, "", threads, nil); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("err = %v, want GitHub's refusal", err)
	}
	got := reqs()
	if len(got) != 3 || !strings.Contains(got[2].Query, "deletePullRequestReview(") {
		t.Fatalf("got %d requests, want start, file comment, delete", len(got))
	}
}

func TestSubmitPendingReviewFileComments(t *testing.T) {
	c, reqs := sequenceServer(t, reviewFileAdded, reviewSubmitted)
	threads := []ReviewThreadDraft{{Path: "c.go", File: true, Body: "about the file"}}
	if err := c.SubmitPendingReview(t.Context(), "PR_1", "PRR_old", core.ReviewEventComment, "", threads, nil); err != nil {
		t.Fatalf("SubmitPendingReview: %v", err)
	}
	got := reqs()
	if len(got) != 2 || !strings.Contains(got[0].Query, "subjectType: FILE") {
		t.Fatalf("got %d requests, first %q, want the file comment and the submit", len(got), got[0].Query)
	}
}

func TestSubmitReviewWithoutDrafts(t *testing.T) {
	c, reqs := sequenceServer(t, reviewAdded, reviewSubmitted)
	if _, err := c.SubmitReview(t.Context(), "PR_1", "abc123", core.ReviewEventComment, "", nil, nil); err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	got := reqs()
	if len(got) != 2 {
		t.Fatalf("got %d requests, want 2 (start, submit)", len(got))
	}
	if _, ok := got[0].Variables["threads"]; ok {
		t.Errorf("start variables = %#v, want no threads", got[0].Variables)
	}
}

func TestSubmitReviewPendingReview(t *testing.T) {
	c, reqs := pullServer(t, "graphql_review_pending.json")
	_, err := c.SubmitReview(t.Context(), "PR_1", "abc123", core.ReviewEventComment, "", reviewThreads, nil)
	if !errors.Is(err, ErrPendingReview) {
		t.Fatalf("err = %v, want one that wraps ErrPendingReview", err)
	}
	if !errors.Is(err, core.ErrConflict) {
		t.Errorf("err = %v, want it to keep GitHub's refusal too", err)
	}
	// Nothing was started, so there is nothing to delete.
	checkPullQuery(t, reqs(), "addPullRequestReview(", map[string]any{
		"pull": "PR_1", "commit": "abc123",
		"threads": []any{
			map[string]any{"path": "a.go", "side": "RIGHT", "line": float64(7), "body": "one line"},
			map[string]any{"path": "b.go", "side": "LEFT", "line": float64(12), "startLine": float64(9), "startSide": "LEFT", "body": "a range"},
		},
	})
}

func TestSubmitReviewOtherStartFailure(t *testing.T) {
	c, reqs := sequenceServer(t, reviewRefused)
	_, err := c.SubmitReview(t.Context(), "PR_1", "abc123", core.ReviewEventComment, "", reviewThreads, nil)
	if err == nil || errors.Is(err, ErrPendingReview) {
		t.Fatalf("err = %v, want a failure that is not ErrPendingReview", err)
	}
	if n := len(reqs()); n != 1 {
		t.Errorf("got %d requests, want 1", n)
	}
}

func TestSubmitReviewFailureDeletesThePendingReview(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answers []string
		replies []ReviewReplyDraft
		// ops are the mutations sent, in order.
		ops []string
	}{
		{"submit", []string{reviewAdded, reviewRefused, reviewDeleted}, nil,
			[]string{"addPullRequestReview(", "submitPullRequestReview(", "deletePullRequestReview("}},
		{"reply", []string{reviewAdded, reviewRefused, reviewDeleted}, reviewReplies,
			[]string{"addPullRequestReview(", "addPullRequestReviewThreadReply(", "deletePullRequestReview("}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, reqs := sequenceServer(t, tc.answers...)
			id, err := c.SubmitReview(t.Context(), "PR_1", "abc123", core.ReviewEventRequestChanges, "no", reviewThreads, tc.replies)
			if err == nil || id != "" {
				t.Fatalf("SubmitReview = %q, %v; want the failure", id, err)
			}
			if !errors.Is(err, core.ErrConflict) {
				t.Errorf("err = %v, want GitHub's refusal", err)
			}
			got := reqs()
			if len(got) != len(tc.ops) {
				t.Fatalf("got %d requests, want %d", len(got), len(tc.ops))
			}
			for i, op := range tc.ops {
				if !strings.Contains(got[i].Query, op) {
					t.Errorf("request %d query does not contain %q:\n%s", i, op, got[i].Query)
				}
			}
			if v := got[len(got)-1].Variables; !reflect.DeepEqual(v, map[string]any{"review": "PRR_pending"}) {
				t.Errorf("delete variables = %#v, want the pending review", v)
			}
		})
	}
}

func TestSubmitReviewJoinsTheFailureOfTheDelete(t *testing.T) {
	// The submit and then the delete are both refused.
	c, _ := sequenceServer(t, reviewAdded, reviewRefused, `{"data":{"result":null},"errors":[{"type":"FORBIDDEN","message":"no delete"}]}`)
	_, err := c.SubmitReview(t.Context(), "PR_1", "abc123", core.ReviewEventComment, "", nil, nil)
	if !errors.Is(err, core.ErrConflict) || !errors.Is(err, core.ErrForbidden) {
		t.Errorf("err = %v, want both the refusal of the submit and of the delete", err)
	}
}

func TestSubmitReviewNoReviewReturned(t *testing.T) {
	c, _ := sequenceServer(t, `{"data":{"result":{"pullRequestReview":null}}}`)
	if _, err := c.SubmitReview(t.Context(), "PR_1", "abc123", core.ReviewEventComment, "", nil, nil); !errors.Is(err, errNoReview) {
		t.Errorf("err = %v, want errNoReview", err)
	}
}
