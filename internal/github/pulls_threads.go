package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// The review threads of a pull request are read a page at a time, each with
// its comments in full, for the diff and the conversation of its modal. The
// detail reads only a summary of them: see getPullQuery.

// threadComments is how many comments of each thread a page reads, the most
// GitHub gives. A thread with more is marked, since a thread that long is
// rare and the rest is on GitHub.
const threadComments = 100

// pullThreadsQuery reads a page of the review threads of a pull request.
// Every field of it is in the oldest Enterprise Server's schema.
//
// Its cost is the requests it makes, a hundredth of a point each, rounded to
// the nearest whole point, with at least one: the pull request, its
// connection of threads and one connection of comments for each thread, so
// at most 102, which is 1 point, for any page. It asks for at most 100
// threads of 100 comments, about 10,000 nodes, well within GitHub's limit
// of 500,000.
var pullThreadsQuery = fmt.Sprintf(`query PullThreads($owner: String!, $name: String!, $number: Int!, $first: Int!, $after: String) {
  `+rateLimitField+`
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: $first, after: $after) {
        totalCount
        pageInfo { hasNextPage endCursor }
        nodes {
          id path line startLine originalLine originalStartLine
          diffSide startDiffSide subjectType
          isResolved isOutdated
          resolvedBy { login name }
          viewerCanResolve viewerCanUnresolve
          comments(first: %d) {
            totalCount
            nodes {
              id
              author { __typename login ... on User { name } }
              createdAt lastEditedAt body url
              replyTo { id }
              pullRequestReview { id }
            }
          }
        }
      }
    }
  }
}`, threadComments)

// pullThread is the JSON shape of a review thread in pullThreadsQuery.
type pullThread struct {
	ID                string `json:"id"`
	Path              string `json:"path"`
	Line              int    `json:"line"`
	StartLine         int    `json:"startLine"`
	OriginalLine      int    `json:"originalLine"`
	OriginalStartLine int    `json:"originalStartLine"`
	DiffSide          string `json:"diffSide"`
	StartDiffSide     string `json:"startDiffSide"`
	SubjectType       string `json:"subjectType"`
	IsResolved        bool   `json:"isResolved"`
	IsOutdated        bool   `json:"isOutdated"`
	// ResolvedBy is null for a thread that isn't resolved, or whose
	// resolver is gone.
	ResolvedBy         *user `json:"resolvedBy"`
	ViewerCanResolve   bool  `json:"viewerCanResolve"`
	ViewerCanUnresolve bool  `json:"viewerCanUnresolve"`
	Comments           struct {
		TotalCount int                 `json:"totalCount"`
		Nodes      []pullThreadComment `json:"nodes"`
	} `json:"comments"`
}

// pullThreadComment is the JSON shape of a comment in pullThreadsQuery.
type pullThreadComment struct {
	ID         string     `json:"id"`
	Author     *actor     `json:"author"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastEdited *time.Time `json:"lastEditedAt"`
	Body       string     `json:"body"`
	URL        string     `json:"url"`
	ReplyTo    *struct {
		ID string `json:"id"`
	} `json:"replyTo"`
	// Review is the review that the comment was made in, null for a comment
	// that was not made in one.
	Review *struct {
		ID string `json:"id"`
	} `json:"pullRequestReview"`
}

func (c pullThreadComment) core() core.ThreadComment {
	out := core.ThreadComment{ID: c.ID, At: c.CreatedAt, Body: c.Body, URL: c.URL}
	if c.Author != nil {
		out.Author = c.Author.core()
	}
	if c.LastEdited != nil {
		out.EditedAt = *c.LastEdited
	}
	if c.ReplyTo != nil {
		out.ReplyTo = c.ReplyTo.ID
	}
	return out
}

// diffSide returns the side that GitHub names, or def for none.
func diffSide(s string, def core.Side) core.Side {
	switch strings.ToUpper(s) {
	case "LEFT":
		return core.SideLeft
	case "RIGHT":
		return core.SideRight
	default:
		return def
	}
}

func (t pullThread) core() core.Thread {
	out := core.Thread{
		ID:                 t.ID,
		Path:               oneLine(t.Path),
		Line:               t.Line,
		StartLine:          t.StartLine,
		OriginalLine:       t.OriginalLine,
		OriginalStartLine:  t.OriginalStartLine,
		Side:               diffSide(t.DiffSide, core.SideRight),
		File:               strings.EqualFold(t.SubjectType, "FILE"),
		Resolved:           t.IsResolved,
		Outdated:           t.IsOutdated,
		ViewerCanResolve:   t.ViewerCanResolve,
		ViewerCanUnresolve: t.ViewerCanUnresolve,
		Comments:           convert(t.Comments.Nodes, pullThreadComment.core),
		TotalComments:      t.Comments.TotalCount,
		MoreComments:       t.Comments.TotalCount > len(t.Comments.Nodes),
	}
	// A thread belongs to the review of its first comment, if it has one.
	if len(t.Comments.Nodes) > 0 && t.Comments.Nodes[0].Review != nil {
		out.ReviewID = t.Comments.Nodes[0].Review.ID
	}
	// A thread on one line starts on the side it ends on.
	out.StartSide = diffSide(t.StartDiffSide, out.Side)
	if t.ResolvedBy != nil {
		out.ResolvedBy = t.ResolvedBy.core()
	}
	return out
}

// The mutations that resolve and unresolve a review thread. Neither is told
// the pull request: a thread's node ID names it. The payload is aliased to
// result, so that one shape decodes both.
const (
	resolveThreadMutation = `mutation ResolveReviewThread($id: ID!) {
  result: resolveReviewThread(input: {threadId: $id}) { thread { id isResolved } }
}`
	unresolveThreadMutation = `mutation UnresolveReviewThread($id: ID!) {
  result: unresolveReviewThread(input: {threadId: $id}) { thread { id isResolved } }
}`
)

// SetReviewThreadResolved resolves the review thread with node ID id, or
// unresolves it if resolved is false. GitHub answers with the thread as it
// left it, and it is an error if that is not the state asked for.
func (c *Client) SetReviewThreadResolved(ctx context.Context, id string, resolved bool) error {
	if resolved {
		return c.setThreadResolved(ctx, resolveThreadMutation, "resolve", id, true)
	}
	return c.setThreadResolved(ctx, unresolveThreadMutation, "unresolve", id, false)
}

func (c *Client) setThreadResolved(ctx context.Context, query, what, id string, resolved bool) error {
	var data struct {
		Result *struct {
			Thread *struct {
				IsResolved bool `json:"isResolved"`
			} `json:"thread"`
		} `json:"result"`
	}
	err := c.Query(ctx, query, map[string]any{"id": id}, &data)
	if err == nil && (data.Result == nil || data.Result.Thread == nil) {
		err = errNoThread
	} else if err == nil && data.Result.Thread.IsResolved != resolved {
		err = errThreadState
	}
	if err != nil {
		return fmt.Errorf("%s review thread %s: %w", what, id, err)
	}
	return nil
}

var (
	// errNoThread is returned when a mutation succeeds without returning the
	// thread, which GitHub should not do.
	errNoThread = errors.New("response has no review thread")
	// errThreadState is returned when GitHub accepts the mutation but leaves
	// the thread as it was.
	errThreadState = errors.New("the thread was not changed")
)

// ListPullRequestThreads returns a page of up to first review threads of
// pull request number of repo, in the order GitHub gives them, each with its
// first 100 comments. Cursor and first work as in ListPullRequests.
func (c *Client) ListPullRequestThreads(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.ThreadPage, error) {
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number, "first": min(max(first, 1), 100)}
	if cursor != "" {
		vars["after"] = cursor
	}
	var data struct {
		Repository *struct {
			PullRequest *struct {
				ReviewThreads struct {
					TotalCount int          `json:"totalCount"`
					PageInfo   pageInfo     `json:"pageInfo"`
					Nodes      []pullThread `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	err := c.Query(ctx, pullThreadsQuery, vars, &data)
	if err == nil && (data.Repository == nil || data.Repository.PullRequest == nil) {
		err = core.ErrNotFound
	}
	if err != nil {
		return core.ThreadPage{}, fmt.Errorf("list review threads of pull request %s#%d: %w", repo, number, err)
	}
	t := data.Repository.PullRequest.ReviewThreads
	return core.ThreadPage{Threads: convert(t.Nodes, pullThread.core), Total: t.TotalCount, Next: t.PageInfo.next()}, nil
}
