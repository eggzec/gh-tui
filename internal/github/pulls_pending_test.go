package github

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// pendingReadAnswer is the answer to the read of a pending review whose first
// page of comments is comments (JSON objects), with a next page if next is not
// empty.
func pendingReadAnswer(author bool, comments, next string) string {
	return fmt.Sprintf(`{"data":{"repository":{"pullRequest":{"reviews":{"nodes":[{
  "id":"PRR_pending","body":"half a summary","viewerDidAuthor":%t,"commit":{"oid":"abc123"},
  "comments":{"pageInfo":%s,"nodes":[%s]}}]}}}}}`, author, pendingPageInfo(next), comments)
}

func pendingPageInfo(next string) string {
	if next == "" {
		return `{"hasNextPage":false,"endCursor":"end"}`
	}
	return fmt.Sprintf(`{"hasNextPage":true,"endCursor":%q}`, next)
}

// pendingPageAnswer is the answer to the read of a later page of comments.
func pendingPageAnswer(comments, next string) string {
	return fmt.Sprintf(`{"data":{"node":{"comments":{"pageInfo":%s,"nodes":[%s]}}}}`, pendingPageInfo(next), comments)
}

const (
	pendingLine = `{"id":"PRRC_1","path":"a.go","line":7,"startLine":null,"originalLine":7,"originalStartLine":null,
  "subjectType":"LINE","outdated":false,"body":"one line","replyTo":null}`
	pendingRange = `{"id":"PRRC_2","path":"b.go","line":12,"startLine":9,"originalLine":12,"originalStartLine":9,
  "subjectType":"LINE","outdated":true,"body":"a range","replyTo":null}`
	pendingReply = `{"id":"PRRC_3","path":"a.go","line":7,"startLine":null,"originalLine":7,"originalStartLine":null,
  "subjectType":"LINE","outdated":false,"body":"a reply","replyTo":{"id":"PRRC_0"}}`
	pendingFile = `{"id":"PRRC_4","path":"c.md","line":null,"startLine":null,"originalLine":null,"originalStartLine":null,
  "subjectType":"FILE","outdated":false,"body":"a file","replyTo":null}`
)

var pendingRepo = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}

func TestPendingReview(t *testing.T) {
	c, reqs := sequenceServer(t, pendingReadAnswer(true, strings.Join([]string{pendingLine, pendingRange, pendingReply, pendingFile}, ","), ""))
	got, ok, err := c.PendingReview(t.Context(), pendingRepo, 42)
	if err != nil || !ok {
		t.Fatalf("PendingReview = %v, %v; want the review", ok, err)
	}
	want := core.PendingReview{
		ID: "PRR_pending", Commit: "abc123", Body: "half a summary",
		Comments: []core.PendingComment{
			{ID: "PRRC_1", Path: "a.go", Line: 7, OriginalLine: 7, Body: "one line"},
			{ID: "PRRC_2", Path: "b.go", Line: 12, StartLine: 9, OriginalLine: 12, OriginalStartLine: 9, Body: "a range", Outdated: true},
			{ID: "PRRC_3", Path: "a.go", Line: 7, OriginalLine: 7, Body: "a reply", ReplyTo: "PRRC_0"},
			{ID: "PRRC_4", Path: "c.md", Body: "a file", File: true},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("review =\n%+v\nwant\n%+v", got, want)
	}
	checkPullQuery(t, reqs(), "reviews(states: [PENDING], first: 1)", map[string]any{"owner": "eggzec", "name": "gh-tui", "number": float64(42)})
}

func TestPendingReviewNone(t *testing.T) {
	c, reqs := sequenceServer(t, `{"data":{"repository":{"pullRequest":{"reviews":{"nodes":[]}}}}}`)
	got, ok, err := c.PendingReview(t.Context(), pendingRepo, 42)
	if err != nil || ok || !reflect.DeepEqual(got, core.PendingReview{}) {
		t.Errorf("PendingReview = %+v, %v, %v; want none", got, ok, err)
	}
	if n := len(reqs()); n != 1 {
		t.Errorf("got %d requests, want 1", n)
	}
}

func TestPendingReviewOfAnotherAuthor(t *testing.T) {
	c, _ := sequenceServer(t, pendingReadAnswer(false, pendingLine, ""))
	if _, ok, err := c.PendingReview(t.Context(), pendingRepo, 42); err != nil || ok {
		t.Errorf("PendingReview = %v, %v; want none, since the review is not the viewer's", ok, err)
	}
}

func TestPendingReviewNotFound(t *testing.T) {
	c, _ := sequenceServer(t, `{"data":{"repository":{"pullRequest":null}}}`)
	if _, _, err := c.PendingReview(t.Context(), pendingRepo, 42); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestPendingReviewPagesThroughEveryComment(t *testing.T) {
	c, reqs := sequenceServer(t,
		pendingReadAnswer(true, pendingLine, "cursor1"),
		pendingPageAnswer(pendingRange, "cursor2"),
		pendingPageAnswer(pendingReply, ""),
	)
	got, ok, err := c.PendingReview(t.Context(), pendingRepo, 42)
	if err != nil || !ok {
		t.Fatalf("PendingReview = %v, %v; want the review", ok, err)
	}
	ids := make([]string, 0, len(got.Comments))
	for _, cm := range got.Comments {
		ids = append(ids, cm.ID)
	}
	if want := []string{"PRRC_1", "PRRC_2", "PRRC_3"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("comments = %q, want %q", ids, want)
	}
	rs := reqs()
	if len(rs) != 3 {
		t.Fatalf("got %d requests, want 3", len(rs))
	}
	for i, after := range []string{"cursor1", "cursor2"} {
		if !strings.Contains(rs[i+1].Query, "node(id: $review)") {
			t.Errorf("request %d is not the read of a page of comments:\n%s", i+1, rs[i+1].Query)
		}
		if want := (map[string]any{"review": "PRR_pending", "after": after}); !reflect.DeepEqual(rs[i+1].Variables, want) {
			t.Errorf("request %d variables = %#v, want %#v", i+1, rs[i+1].Variables, want)
		}
	}
}

func TestPendingReviewPageFailure(t *testing.T) {
	c, _ := sequenceServer(t, pendingReadAnswer(true, pendingLine, "cursor1"), reviewRefused)
	if _, ok, err := c.PendingReview(t.Context(), pendingRepo, 42); err == nil || ok {
		t.Errorf("PendingReview = %v, %v; want the failure and no review", ok, err)
	}
}

func TestPendingReviewStopsOnARepeatedCursor(t *testing.T) {
	c, reqs := sequenceServer(t, pendingReadAnswer(true, pendingLine, "cursor1"), pendingPageAnswer(pendingRange, "cursor1"))
	got, ok, err := c.PendingReview(t.Context(), pendingRepo, 42)
	if err != nil || !ok || len(got.Comments) != 2 {
		t.Fatalf("PendingReview = %+v, %v, %v; want two comments", got, ok, err)
	}
	if n := len(reqs()); n != 2 {
		t.Errorf("got %d requests, want 2: a page that repeats its cursor ends the read", n)
	}
}

func TestSubmitPendingReview(t *testing.T) {
	c, reqs := sequenceServer(t, `{"data":{"result":{"thread":{"id":"PRRT_new"}}}}`, reviewReplied, reviewSubmitted)
	threads := reviewThreads[:1]
	err := c.SubmitPendingReview(t.Context(), "PR_1", "PRR_pending", core.ReviewEventApprove, "looks good", threads, reviewReplies)
	if err != nil {
		t.Fatalf("SubmitPendingReview: %v", err)
	}
	got := reqs()
	if len(got) != 3 {
		t.Fatalf("got %d requests, want 3 (thread, reply, submit)", len(got))
	}
	want := []struct {
		op   string
		vars map[string]any
	}{
		{"addPullRequestReviewThread(", map[string]any{"pull": "PR_1", "review": "PRR_pending", "path": "a.go", "side": "RIGHT", "line": float64(7), "body": "one line"}},
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
}

func TestSubmitPendingReviewRange(t *testing.T) {
	c, reqs := sequenceServer(t, `{"data":{"result":{"thread":{"id":"PRRT_new"}}}}`, reviewSubmitted)
	if err := c.SubmitPendingReview(t.Context(), "PR_1", "PRR_pending", core.ReviewEventComment, "", reviewThreads[1:], nil); err != nil {
		t.Fatalf("SubmitPendingReview: %v", err)
	}
	want := map[string]any{"pull": "PR_1", "review": "PRR_pending", "path": "b.go", "side": "LEFT", "line": float64(12), "startLine": float64(9), "startSide": "LEFT", "body": "a range"}
	if got := reqs()[0].Variables; !reflect.DeepEqual(got, want) {
		t.Errorf("variables = %#v, want %#v", got, want)
	}
}

// A refused step leaves what was added in the pending review, and never
// deletes it: it holds comments that were not made here.
func TestSubmitPendingReviewFailureDeletesNothing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answers []string
		replies []ReviewReplyDraft
		ops     []string
	}{
		{"thread", []string{reviewRefused}, nil, []string{"addPullRequestReviewThread("}},
		{"second thread", []string{`{"data":{"result":{"thread":{"id":"PRRT_new"}}}}`, reviewRefused}, nil,
			[]string{"addPullRequestReviewThread(", "addPullRequestReviewThread("}},
		{"reply", []string{`{"data":{"result":{"thread":{"id":"PRRT_new"}}}}`, `{"data":{"result":{"thread":{"id":"PRRT_new"}}}}`, reviewRefused}, reviewReplies,
			[]string{"addPullRequestReviewThread(", "addPullRequestReviewThread(", "addPullRequestReviewThreadReply("}},
		{"submit", []string{`{"data":{"result":{"thread":{"id":"PRRT_new"}}}}`, `{"data":{"result":{"thread":{"id":"PRRT_new"}}}}`, reviewRefused}, nil,
			[]string{"addPullRequestReviewThread(", "addPullRequestReviewThread(", "submitPullRequestReview("}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, reqs := sequenceServer(t, tc.answers...)
			err := c.SubmitPendingReview(t.Context(), "PR_1", "PRR_pending", core.ReviewEventComment, "", reviewThreads, tc.replies)
			if !errors.Is(err, core.ErrConflict) {
				t.Fatalf("err = %v, want GitHub's refusal", err)
			}
			got := reqs()
			if len(got) != len(tc.ops) {
				t.Fatalf("got %d requests, want %d", len(got), len(tc.ops))
			}
			for i, op := range tc.ops {
				if !strings.Contains(got[i].Query, op) {
					t.Errorf("request %d query does not contain %q:\n%s", i, op, got[i].Query)
				}
				if strings.Contains(got[i].Query, "delete") {
					t.Errorf("request %d deletes:\n%s", i, got[i].Query)
				}
			}
		})
	}
}
