package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// pendingComments is how many comments of the pending review a read takes at
// once, the most GitHub gives.
const pendingComments = 100

// pendingCommentFields are the fields of a comment of a pending review, as
// both reads of them ask for. Every one is in the oldest Enterprise Server's
// schema.
const pendingCommentFields = `id path line startLine originalLine originalStartLine subjectType outdated body replyTo { id }`

// pendingReviewQuery reads the viewer's pending review of a pull request,
// with its first page of comments. Only its author sees a pending review, so
// asking for the pending ones returns the viewer's own, or none. Its cost is
// that of one request for the pull request and one for the connection of
// comments, which is 1 point.
var pendingReviewQuery = fmt.Sprintf(`query PullPendingReview($owner: String!, $name: String!, $number: Int!) {
  `+rateLimitField+`
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviews(states: [PENDING], first: 1) {
        nodes {
          id body viewerDidAuthor
          commit { oid }
          comments(first: %[1]d) {
            pageInfo { hasNextPage endCursor }
            nodes { %[2]s }
          }
        }
      }
    }
  }
}`, pendingComments, pendingCommentFields)

// pendingCommentsQuery reads a page of the comments of a pending review, after
// the first page, which the review's own read has.
var pendingCommentsQuery = fmt.Sprintf(`query PullPendingComments($review: ID!, $after: String) {
  `+rateLimitField+`
  node(id: $review) {
    ... on PullRequestReview {
      comments(first: %[1]d, after: $after) {
        pageInfo { hasNextPage endCursor }
        nodes { %[2]s }
      }
    }
  }
}`, pendingComments, pendingCommentFields)

// A thread is added to a pending review that exists already by this mutation:
// the review input that starts one takes its threads, and this one is for
// adding to it. The payload is aliased to result, as the review's are.
const addReviewThreadMutation = `mutation AddPullRequestReviewThread($pull: ID!, $review: ID!, $path: String!, $line: Int!, $side: DiffSide, $startLine: Int, $startSide: DiffSide, $body: String!) {
  result: addPullRequestReviewThread(input: {pullRequestId: $pull, pullRequestReviewId: $review, path: $path, line: $line, side: $side, startLine: $startLine, startSide: $startSide, body: $body}) { thread { id } }
}`

// pendingComment is the JSON shape of a comment in the pending reads.
type pendingComment struct {
	ID                string `json:"id"`
	Path              string `json:"path"`
	Line              int    `json:"line"`
	StartLine         int    `json:"startLine"`
	OriginalLine      int    `json:"originalLine"`
	OriginalStartLine int    `json:"originalStartLine"`
	SubjectType       string `json:"subjectType"`
	Outdated          bool   `json:"outdated"`
	Body              string `json:"body"`
	ReplyTo           *struct {
		ID string `json:"id"`
	} `json:"replyTo"`
}

func (c pendingComment) core() core.PendingComment {
	out := core.PendingComment{
		ID: c.ID, Path: oneLine(c.Path),
		Line: c.Line, StartLine: c.StartLine,
		OriginalLine: c.OriginalLine, OriginalStartLine: c.OriginalStartLine,
		Body: c.Body, Outdated: c.Outdated,
		File: strings.EqualFold(c.SubjectType, "FILE"),
	}
	if c.ReplyTo != nil {
		out.ReplyTo = c.ReplyTo.ID
	}
	return out
}

// pendingConn is a page of the comments of a pending review.
type pendingConn struct {
	PageInfo pageInfo         `json:"pageInfo"`
	Nodes    []pendingComment `json:"nodes"`
}

// PendingReview returns the viewer's pending review of pull request number of
// repo, with every one of its comments, which it reads a page at a time. The
// bool is false when the viewer has none.
func (c *Client) PendingReview(ctx context.Context, repo core.RepoRef, number int) (core.PendingReview, bool, error) {
	var data struct {
		Repository *struct {
			PullRequest *struct {
				Reviews struct {
					Nodes []struct {
						ID              string `json:"id"`
						Body            string `json:"body"`
						ViewerDidAuthor bool   `json:"viewerDidAuthor"`
						Commit          *struct {
							OID string `json:"oid"`
						} `json:"commit"`
						Comments pendingConn `json:"comments"`
					} `json:"nodes"`
				} `json:"reviews"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	err := c.Query(ctx, pendingReviewQuery, vars, &data)
	if err == nil && (data.Repository == nil || data.Repository.PullRequest == nil) {
		err = core.ErrNotFound
	}
	if err != nil {
		return core.PendingReview{}, false, fmt.Errorf("read pending review of pull request %s#%d: %w", repo, number, err)
	}
	nodes := data.Repository.PullRequest.Reviews.Nodes
	// Someone else's pending review is not the viewer's to add to.
	if len(nodes) == 0 || !nodes[0].ViewerDidAuthor {
		return core.PendingReview{}, false, nil
	}
	n := nodes[0]
	out := core.PendingReview{ID: n.ID, Body: n.Body, Comments: convert(n.Comments.Nodes, pendingComment.core)}
	if n.Commit != nil {
		out.Commit = n.Commit.OID
	}
	after := n.Comments.PageInfo.next()
	for after != "" {
		more, err := c.pendingCommentsPage(ctx, n.ID, after)
		if err != nil {
			return core.PendingReview{}, false, fmt.Errorf("read pending review of pull request %s#%d: %w", repo, number, err)
		}
		out.Comments = append(out.Comments, convert(more.Nodes, pendingComment.core)...)
		next := more.PageInfo.next()
		if next == after {
			break
		}
		after = next
	}
	return out, true, nil
}

// pendingCommentsPage reads the page of comments of review after the cursor.
func (c *Client) pendingCommentsPage(ctx context.Context, review, after string) (pendingConn, error) {
	var data struct {
		Node *struct {
			Comments *pendingConn `json:"comments"`
		} `json:"node"`
	}
	if err := c.Query(ctx, pendingCommentsQuery, map[string]any{"review": review, "after": after}, &data); err != nil {
		return pendingConn{}, err
	}
	if data.Node == nil || data.Node.Comments == nil {
		return pendingConn{}, core.ErrNotFound
	}
	return *data.Node.Comments, nil
}

// SubmitPendingReview adds threads and replies to the pending review reviewID
// of the pull request with node ID pullID, and submits it with the summary
// body and the verdict event. Unlike SubmitReview it never deletes the
// review, which holds comments that were not made here. If a step fails, the
// comments added before it stay in the review, on GitHub, and the error is
// returned.
func (c *Client) SubmitPendingReview(
	ctx context.Context, pullID, reviewID string, event core.ReviewEvent, body string,
	threads []ReviewThreadDraft, replies []ReviewReplyDraft,
) error {
	for _, t := range threads {
		vars := map[string]any{"pull": pullID, "review": reviewID, "path": t.Path, "line": t.Line, "side": t.Side, "body": t.Body}
		if t.StartLine > 0 {
			vars["startLine"], vars["startSide"] = t.StartLine, t.Side
		}
		if err := c.Query(ctx, addReviewThreadMutation, vars, nil); err != nil {
			return fmt.Errorf("add thread on %s to review %s: %w", t.Path, reviewID, err)
		}
	}
	return c.finishReview(ctx, pullID, reviewID, event, body, replies)
}
