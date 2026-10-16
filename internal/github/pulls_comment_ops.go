package github

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// The mutations that edit a pull request's text, reply to a review thread, and
// edit or delete the comments of a pull request. Each aliases its payload to
// result, so that one shape decodes it.
var (
	editPullMutation = `mutation EditPullRequest($id: ID!, $title: String, $body: String) {
  result: updatePullRequest(input: {pullRequestId: $id, title: $title, body: $body}) {
    pullRequest { ...pullFields body lastEditedAt }
  }
}
` + pullFields
	// The reply reads its comment as pullThreadsQuery does, to decode alike.
	// A null review replies at once, outside any pending review.
	replyThreadMutation = `mutation ReplyToReviewThread($thread: ID!, $review: ID, $body: String!) {
  result: addPullRequestReviewThreadReply(input: {pullRequestReviewThreadId: $thread, pullRequestReviewId: $review, body: $body}) {
    comment {
      ` + pullThreadCommentFields + `
    }
  }
}`
)

const (
	editIssueCommentMutation = `mutation EditIssueComment($id: ID!, $body: String!) {
  result: updateIssueComment(input: {id: $id, body: $body}) { issueComment { id body lastEditedAt } }
}`
	editReviewCommentMutation = `mutation EditReviewComment($id: ID!, $body: String!) {
  result: updatePullRequestReviewComment(input: {pullRequestReviewCommentId: $id, body: $body}) { pullRequestReviewComment { id body lastEditedAt } }
}`
	editReviewMutation = `mutation EditReviewBody($id: ID!, $body: String!) {
  result: updatePullRequestReview(input: {pullRequestReviewId: $id, body: $body}) { pullRequestReview { id body lastEditedAt } }
}`
	deleteIssueCommentMutation = `mutation DeleteIssueComment($id: ID!) {
  result: deleteIssueComment(input: {id: $id}) { clientMutationId }
}`
	deleteReviewCommentMutation = `mutation DeleteReviewComment($id: ID!) {
  result: deletePullRequestReviewComment(input: {id: $id}) { pullRequestReviewComment { id } }
}`
)

// errNoComment is returned when a mutation of a comment succeeds without
// returning it, which GitHub should not do.
var errNoComment = errors.New("response has no comment")

// pullCommentEdit is the JSON shape of a comment, a reply or a review summary
// that an edit returns.
type pullCommentEdit struct {
	ID         string     `json:"id"`
	Body       string     `json:"body"`
	LastEdited *time.Time `json:"lastEditedAt"`
}

func (e pullCommentEdit) core() core.Edited {
	out := core.Edited{ID: e.ID, Body: e.Body}
	if e.LastEdited != nil {
		out.At = *e.LastEdited
	}
	return out
}

// UpdatePullRequestText sets the title and the description of the pull request
// with node ID pullID, and returns it with its description set, and when that
// was last edited. GitHub keeps what the mutation leaves out, so both are sent.
func (c *Client) UpdatePullRequestText(ctx context.Context, pullID, title, body string) (core.PullRequest, time.Time, error) {
	var data struct {
		Result *struct {
			PullRequest *struct {
				pull
				Body       string     `json:"body"`
				LastEdited *time.Time `json:"lastEditedAt"`
			} `json:"pullRequest"`
		} `json:"result"`
	}
	err := c.Query(ctx, editPullMutation, map[string]any{"id": pullID, "title": title, "body": body}, &data)
	if err == nil && (data.Result == nil || data.Result.PullRequest == nil) {
		err = errNoPull
	}
	if err != nil {
		return core.PullRequest{}, time.Time{}, fmt.Errorf("edit pull request %s: %w", pullID, err)
	}
	p := data.Result.PullRequest
	out := p.core()
	out.Body = p.Body
	var at time.Time
	if p.LastEdited != nil {
		at = *p.LastEdited
	}
	return out, at, nil
}

// ReplyToReviewThread adds a reply with body to the review thread with node ID
// threadID and returns it. A reviewID joins the reply to that pending review of
// the viewer; "" posts it at once.
func (c *Client) ReplyToReviewThread(ctx context.Context, threadID, reviewID, body string) (core.ThreadComment, error) {
	vars := map[string]any{"thread": threadID, "review": nil, "body": body}
	if reviewID != "" {
		vars["review"] = reviewID
	}
	var data struct {
		Result *struct {
			Comment *pullThreadComment `json:"comment"`
		} `json:"result"`
	}
	err := c.Query(ctx, replyThreadMutation, vars, &data)
	if err == nil && (data.Result == nil || data.Result.Comment == nil) {
		err = errNoComment
	}
	if err != nil {
		// A reply outside the pending review may be refused for the pending
		// review there is, which the caller joins instead.
		if reviewID == "" && isPendingReview(err) {
			err = fmt.Errorf("%w: %w", ErrPendingReview, err)
		}
		return core.ThreadComment{}, fmt.Errorf("reply to review thread %s: %w", threadID, err)
	}
	return data.Result.Comment.core(), nil
}

// EditIssueComment sets the body of the comment with node ID id, on the
// conversation of a pull request.
func (c *Client) EditIssueComment(ctx context.Context, id, body string) (core.Edited, error) {
	return editComment(ctx, c, editIssueCommentMutation, id, body, func(r *pullCommentEditResult) *pullCommentEdit { return r.IssueComment })
}

// EditReviewComment sets the body of the review comment, or reply, with node
// ID id.
func (c *Client) EditReviewComment(ctx context.Context, id, body string) (core.Edited, error) {
	return editComment(ctx, c, editReviewCommentMutation, id, body, func(r *pullCommentEditResult) *pullCommentEdit { return r.ReviewComment })
}

// EditReview sets the summary of the review with node ID id.
func (c *Client) EditReview(ctx context.Context, id, body string) (core.Edited, error) {
	return editComment(ctx, c, editReviewMutation, id, body, func(r *pullCommentEditResult) *pullCommentEdit { return r.Review })
}

// pullCommentEditResult is the payload of an edit: the node is under the name
// of its kind.
type pullCommentEditResult struct {
	IssueComment  *pullCommentEdit `json:"issueComment"`
	ReviewComment *pullCommentEdit `json:"pullRequestReviewComment"`
	Review        *pullCommentEdit `json:"pullRequestReview"`
}

// editComment sends an edit and picks the edited node out of its payload.
func editComment(ctx context.Context, c *Client, query, id, body string, node func(*pullCommentEditResult) *pullCommentEdit) (core.Edited, error) {
	var data struct {
		Result *pullCommentEditResult `json:"result"`
	}
	err := c.Query(ctx, query, map[string]any{"id": id, "body": body}, &data)
	var edited *pullCommentEdit
	if err == nil && data.Result != nil {
		edited = node(data.Result)
	}
	if err == nil && edited == nil {
		err = errNoComment
	}
	if err != nil {
		return core.Edited{}, fmt.Errorf("edit comment %s: %w", id, err)
	}
	return edited.core(), nil
}

// DeleteIssueComment deletes the comment with node ID id from the
// conversation of a pull request.
func (c *Client) DeleteIssueComment(ctx context.Context, id string) error {
	return c.deleteComment(ctx, deleteIssueCommentMutation, id)
}

// DeleteReviewComment deletes the review comment, or reply, with node ID id.
// GitHub deletes only that comment: the replies to it stay.
func (c *Client) DeleteReviewComment(ctx context.Context, id string) error {
	return c.deleteComment(ctx, deleteReviewCommentMutation, id)
}

func (c *Client) deleteComment(ctx context.Context, query, id string) error {
	var data struct {
		Result *struct{} `json:"result"`
	}
	err := c.Query(ctx, query, map[string]any{"id": id}, &data)
	if err == nil && data.Result == nil {
		err = errNoComment
	}
	if err != nil {
		return fmt.Errorf("delete comment %s: %w", id, err)
	}
	return nil
}
