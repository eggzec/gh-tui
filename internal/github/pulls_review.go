package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// ReviewThreadDraft is a comment on a line, or on a range of lines of one
// side of a file, that a review starts a thread with.
type ReviewThreadDraft struct {
	Path string
	// Side is the side of the diff as GitHub spells it: LEFT for a deleted
	// line, RIGHT otherwise.
	Side string
	// Line is the line, or the last of a range, and StartLine the first of a
	// range, or 0 for one line.
	Line      int
	StartLine int
	Body      string
}

// ReviewReplyDraft is a reply to an existing review thread, sent with a
// review.
type ReviewReplyDraft struct {
	// ThreadID is the node ID of the thread.
	ThreadID string
	Body     string
}

// ErrPendingReview is what SubmitReview's error wraps when the viewer already
// has a pending review on the pull request, which GitHub allows only one of.
var ErrPendingReview = errors.New("you have a pending review on this pull request")

// A review is sent in three steps, so that every comment of it lands in one
// review: a pending one (no event) that holds the threads, replies to existing
// threads added to it, and its submission. The payloads are aliased to
// result, so that one shape decodes them.
const (
	addReviewMutation = `mutation AddPullRequestReview($pull: ID!, $commit: GitObjectID, $threads: [DraftPullRequestReviewThread]) {
  result: addPullRequestReview(input: {pullRequestId: $pull, commitOID: $commit, threads: $threads}) { pullRequestReview { id } }
}`
	addReviewReplyMutation = `mutation AddPullRequestReviewThreadReply($review: ID!, $thread: ID!, $body: String!) {
  result: addPullRequestReviewThreadReply(input: {pullRequestReviewId: $review, pullRequestReviewThreadId: $thread, body: $body}) { comment { id } }
}`
	submitReviewMutation = `mutation SubmitPullRequestReview($pull: ID!, $review: ID!, $event: PullRequestReviewEvent!, $body: String) {
  result: submitPullRequestReview(input: {pullRequestId: $pull, pullRequestReviewId: $review, event: $event, body: $body}) { pullRequestReview { id } }
}`
	deleteReviewMutation = `mutation DeletePullRequestReview($review: ID!) {
  result: deletePullRequestReview(input: {pullRequestReviewId: $review}) { pullRequestReview { id } }
}`
)

// errNoReview is returned when a mutation succeeds without returning the
// review, which GitHub should not do.
var errNoReview = errors.New("response has no review")

// cleanupTimeout bounds the deletion of a pending review that a failed
// submission leaves behind.
const cleanupTimeout = 10 * time.Second

// SubmitReview sends threads and replies as one review of the pull request
// with node ID pullID, on commit commitOID (the head the user saw), with the
// summary body and the verdict event, and returns the review's node ID.
//
// It makes a pending review with the threads, adds each reply to it, and
// submits it. If a reply or the submission fails, the pending review is
// deleted, so that nothing half-sent lingers on GitHub, and the error is
// returned (joined with the deletion's, if that failed too). If the viewer
// already has a pending review, which GitHub allows one of, the error wraps
// ErrPendingReview and nothing was sent.
func (c *Client) SubmitReview(
	ctx context.Context, pullID, commitOID string, event core.ReviewEvent, body string,
	threads []ReviewThreadDraft, replies []ReviewReplyDraft,
) (string, error) {
	reviewID, err := c.addReview(ctx, pullID, commitOID, threads)
	if err != nil {
		return "", err
	}
	if err := c.finishReview(ctx, pullID, reviewID, event, body, replies); err != nil {
		// The caller's context may be what failed the step, and the deletion
		// must still go out.
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		defer cancel()
		if derr := c.deleteReview(dctx, reviewID); derr != nil {
			err = errors.Join(err, derr)
		}
		return "", err
	}
	return reviewID, nil
}

// addReview starts a pending review with threads and returns its ID.
func (c *Client) addReview(ctx context.Context, pullID, commitOID string, threads []ReviewThreadDraft) (string, error) {
	vars := map[string]any{"pull": pullID}
	if commitOID != "" {
		vars["commit"] = commitOID
	}
	if len(threads) > 0 {
		vars["threads"] = reviewThreadInputs(threads)
	}
	id, err := c.reviewMutation(ctx, addReviewMutation, vars)
	if err != nil {
		if isPendingReview(err) {
			err = fmt.Errorf("%w: %w", ErrPendingReview, err)
		}
		return "", fmt.Errorf("start review of %s: %w", pullID, err)
	}
	return id, nil
}

// finishReview adds replies to the pending review and submits it.
func (c *Client) finishReview(ctx context.Context, pullID, reviewID string, event core.ReviewEvent, body string, replies []ReviewReplyDraft) error {
	for _, r := range replies {
		vars := map[string]any{"review": reviewID, "thread": r.ThreadID, "body": r.Body}
		if err := c.Query(ctx, addReviewReplyMutation, vars, nil); err != nil {
			return fmt.Errorf("reply to review thread %s: %w", r.ThreadID, err)
		}
	}
	vars := map[string]any{"pull": pullID, "review": reviewID, "event": string(event), "body": body}
	if _, err := c.reviewMutation(ctx, submitReviewMutation, vars); err != nil {
		return fmt.Errorf("submit review %s: %w", reviewID, err)
	}
	return nil
}

// deleteReview deletes the pending review reviewID.
func (c *Client) deleteReview(ctx context.Context, reviewID string) error {
	if _, err := c.reviewMutation(ctx, deleteReviewMutation, map[string]any{"review": reviewID}); err != nil {
		return fmt.Errorf("delete pending review %s: %w", reviewID, err)
	}
	return nil
}

// reviewMutation runs a mutation whose result holds a pullRequestReview and
// returns that review's ID.
func (c *Client) reviewMutation(ctx context.Context, query string, vars map[string]any) (string, error) {
	var data struct {
		Result *struct {
			PullRequestReview *struct {
				ID string `json:"id"`
			} `json:"pullRequestReview"`
		} `json:"result"`
	}
	if err := c.Query(ctx, query, vars, &data); err != nil {
		return "", err
	}
	if data.Result == nil || data.Result.PullRequestReview == nil || data.Result.PullRequestReview.ID == "" {
		return "", errNoReview
	}
	return data.Result.PullRequestReview.ID, nil
}

// reviewThreadInputs are threads as the DraftPullRequestReviewThread input
// has them. A range starts on the side it ends on.
func reviewThreadInputs(threads []ReviewThreadDraft) []map[string]any {
	out := make([]map[string]any, len(threads))
	for i, t := range threads {
		in := map[string]any{"path": t.Path, "line": t.Line, "side": t.Side, "body": t.Body}
		if t.StartLine > 0 {
			in["startLine"], in["startSide"] = t.StartLine, t.Side
		}
		out[i] = in
	}
	return out
}

// isPendingReview reports whether err is GitHub's refusal to start a review
// while the viewer has a pending one.
func isPendingReview(err error) bool {
	var ge *GraphQLError
	if !errors.As(err, &ge) {
		return false
	}
	for _, item := range ge.Errors {
		if strings.Contains(strings.ToLower(item.Message), "pending review") {
			return true
		}
	}
	return false
}
