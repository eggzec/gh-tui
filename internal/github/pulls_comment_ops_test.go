package github

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestUpdatePullRequestText(t *testing.T) {
	c, reqs := pullServer(t, "graphql_comment_ops_edit_pull.json")
	p, at, err := c.UpdatePullRequestText(t.Context(), "PR_1", "New title", "New description")
	if err != nil {
		t.Fatalf("UpdatePullRequestText: %v", err)
	}
	checkPullQuery(t, reqs(), "updatePullRequest(input: {pullRequestId: $id, title: $title, body: $body})", map[string]any{
		"id": "PR_1", "title": "New title", "body": "New description",
	})
	if p.Number != 42 || p.Title != "New title" {
		t.Errorf("pull = #%d %q, want #42 \"New title\"", p.Number, p.Title)
	}
	if want := "New description, as GitHub keeps it"; p.Body != want {
		t.Errorf("body = %q, want %q", p.Body, want)
	}
	if want := time.Date(2025, 2, 1, 10, 20, 30, 0, time.UTC); !at.Equal(want) {
		t.Errorf("edited at = %v, want %v", at, want)
	}
}

func TestReplyToReviewThread(t *testing.T) {
	tests := []struct {
		name   string
		review string
		want   any
	}{
		{"into a pending review", "PRR_pending", "PRR_pending"},
		{"at once", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := pullServer(t, "graphql_comment_ops_reply.json")
			got, err := c.ReplyToReviewThread(t.Context(), "PRRT_1", tt.review, "a reply")
			if err != nil {
				t.Fatalf("ReplyToReviewThread: %v", err)
			}
			checkPullQuery(t, reqs(), "addPullRequestReviewThreadReply(input: {pullRequestReviewThreadId: $thread, pullRequestReviewId: $review, body: $body})", map[string]any{
				"thread": "PRRT_1", "review": tt.want, "body": "a reply",
			})
			// The key is sent even when it is null, not left out.
			if _, ok := reqs()[0].Variables["review"]; !ok {
				t.Error("variables have no review key")
			}
			want := core.ThreadComment{
				ID: "PRRC_reply", Author: core.User{Login: "monalisa", Name: "Mona Lisa"},
				At:   time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC),
				Body: "a reply", URL: "https://github.com/eggzec/gh-tui/pull/42#discussion_r9",
				ReplyTo: "PRRC_first", Pending: true,
				Path: "a.go", DiffHunk: "@@ -1 +1 @@", Commit: "abc", OriginalCommit: "abc",
				Line: 7, OriginalLine: 7, ViewerCanUpdate: true, ViewerCanDelete: true,
			}
			if got != want {
				t.Errorf("comment = %+v, want %+v", got, want)
			}
		})
	}
}

func TestReplyQueryReadsThreadComments(t *testing.T) {
	if !strings.Contains(replyThreadMutation, pullThreadCommentFields) || !strings.Contains(pullThreadsQuery, pullThreadCommentFields) {
		t.Error("the reply and the threads query don't read the same fields of a comment")
	}
}

func TestEditComments(t *testing.T) {
	at := func(s int) time.Time { return time.Date(2025, 2, 1, 10, 20, s, 0, time.UTC) }
	tests := []struct {
		name    string
		fixture string
		call    func(*Client, context.Context) (core.Edited, error)
		input   string
		want    core.Edited
	}{
		{
			"issue comment", "edit_issue_comment",
			func(c *Client, ctx context.Context) (core.Edited, error) {
				return c.EditIssueComment(ctx, "IC_1", "new")
			},
			"updateIssueComment(input: {id: $id, body: $body})",
			core.Edited{ID: "IC_1", Body: "edited issue comment", At: at(30)},
		},
		{
			"review comment", "edit_review_comment",
			func(c *Client, ctx context.Context) (core.Edited, error) {
				return c.EditReviewComment(ctx, "PRRC_1", "new")
			},
			"updatePullRequestReviewComment(input: {pullRequestReviewCommentId: $id, body: $body})",
			core.Edited{ID: "PRRC_1", Body: "edited review comment", At: at(31)},
		},
		{
			"review", "edit_review",
			func(c *Client, ctx context.Context) (core.Edited, error) { return c.EditReview(ctx, "PRR_1", "new") },
			"updatePullRequestReview(input: {pullRequestReviewId: $id, body: $body})",
			core.Edited{ID: "PRR_1", Body: "edited summary", At: at(32)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := pullServer(t, "graphql_comment_ops_"+tt.fixture+".json")
			got, err := tt.call(c, t.Context())
			if err != nil {
				t.Fatalf("edit: %v", err)
			}
			checkPullQuery(t, reqs(), tt.input, map[string]any{"id": tt.want.ID, "body": "new"})
			if got != tt.want {
				t.Errorf("edited = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDeleteComments(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		call    func(*Client, context.Context) error
		input   string
	}{
		{
			"issue comment", "delete_issue",
			func(c *Client, ctx context.Context) error { return c.DeleteIssueComment(ctx, "IC_1") },
			"deleteIssueComment(input: {id: $id})",
		},
		{
			"review comment", "delete_review",
			func(c *Client, ctx context.Context) error { return c.DeleteReviewComment(ctx, "IC_1") },
			"deletePullRequestReviewComment(input: {id: $id})",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := pullServer(t, "graphql_comment_ops_"+tt.fixture+".json")
			if err := tt.call(c, t.Context()); err != nil {
				t.Fatalf("delete: %v", err)
			}
			checkPullQuery(t, reqs(), tt.input, map[string]any{"id": "IC_1"})
		})
	}
}

func TestCommentOpsRefused(t *testing.T) {
	ops := map[string]func(*Client, context.Context) error{
		"edit pull request PR_1": func(c *Client, ctx context.Context) error {
			_, _, err := c.UpdatePullRequestText(ctx, "PR_1", "t", "b")
			return err
		},
		"reply to review thread PRRT_1": func(c *Client, ctx context.Context) error {
			_, err := c.ReplyToReviewThread(ctx, "PRRT_1", "", "b")
			return err
		},
		"edit comment IC_1": func(c *Client, ctx context.Context) error {
			_, err := c.EditIssueComment(ctx, "IC_1", "b")
			return err
		},
		"delete comment IC_1": func(c *Client, ctx context.Context) error { return c.DeleteIssueComment(ctx, "IC_1") },
	}
	for want, op := range ops {
		t.Run(want, func(t *testing.T) {
			c, _ := pullServer(t, "graphql_comment_ops_refused.json")
			err := op(c, t.Context())
			if err == nil || !strings.HasPrefix(err.Error(), want+": ") || !strings.Contains(err.Error(), "Resource not accessible") {
				t.Errorf("err = %v, want %q wrapping GitHub's message", err, want)
			}
		})
	}
}

func TestReplyRefusedForAPendingReview(t *testing.T) {
	tests := []struct {
		name   string
		review string
		want   bool
	}{
		{"at once", "", true},
		// Into the pending review itself, the refusal is not about there being one.
		{"into a pending review", "PRR_1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := pullServer(t, "graphql_comment_ops_pending.json")
			_, err := c.ReplyToReviewThread(t.Context(), "PRRT_1", tt.review, "b")
			if err == nil || !strings.Contains(err.Error(), "pending review") {
				t.Fatalf("err = %v, want GitHub's message", err)
			}
			if got := errors.Is(err, ErrPendingReview); got != tt.want {
				t.Errorf("errors.Is(err, ErrPendingReview) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCommentOpsNullPayload(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client, context.Context) error
		want error
	}{
		{"edit pull", func(c *Client, ctx context.Context) error {
			_, _, err := c.UpdatePullRequestText(ctx, "PR_1", "t", "b")
			return err
		}, errNoPull},
		{"reply", func(c *Client, ctx context.Context) error {
			_, err := c.ReplyToReviewThread(ctx, "PRRT_1", "", "b")
			return err
		}, errNoComment},
		{"edit comment", func(c *Client, ctx context.Context) error {
			_, err := c.EditReviewComment(ctx, "PRRC_1", "b")
			return err
		}, errNoComment},
		{"delete comment", func(c *Client, ctx context.Context) error { return c.DeleteReviewComment(ctx, "PRRC_1") }, errNoComment},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := pullServer(t, "graphql_comment_ops_null.json")
			if err := tt.call(c, t.Context()); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
