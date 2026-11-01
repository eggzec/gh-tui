package github

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/eggzec/gh-tui/internal/core"
)

// issueTimelineQuery reads a page of the timeline of an issue. It selects
// the members of the issue's union that pullTimelineQuery shares, so
// timelineNode maps them.
var issueTimelineQuery = fmt.Sprintf(`query IssueTimeline($owner: String!, $name: String!, $number: Int!, $first: Int!, $after: String) {
  %s
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      page: timelineItems(first: $first, after: $after, itemTypes: [
        ISSUE_COMMENT LABELED_EVENT UNLABELED_EVENT ASSIGNED_EVENT UNASSIGNED_EVENT
        CLOSED_EVENT REOPENED_EVENT RENAMED_TITLE_EVENT CROSS_REFERENCED_EVENT
        LOCKED_EVENT UNLOCKED_EVENT MILESTONED_EVENT DEMILESTONED_EVENT
      ]) {
        nodes {
          __typename
          ... on IssueComment {
            id author { __typename login avatarUrl ... on User { name } }
            body createdAt updatedAt url viewerCanUpdate viewerCanDelete
          }
          ... on LabeledEvent { actor { __typename login ... on User { name } } createdAt label { name color description } }
          ... on UnlabeledEvent { actor { __typename login ... on User { name } } createdAt label { name color description } }
          ... on AssignedEvent {
            actor { __typename login ... on User { name } } createdAt
            assignee { __typename ... on Actor { login } ... on User { name } }
          }
          ... on UnassignedEvent {
            actor { __typename login ... on User { name } } createdAt
            assignee { __typename ... on Actor { login } ... on User { name } }
          }
          ... on ClosedEvent { actor { __typename login ... on User { name } } createdAt }
          ... on ReopenedEvent { actor { __typename login ... on User { name } } createdAt }
          ... on RenamedTitleEvent {
            actor { __typename login ... on User { name } } createdAt
            previousTitle currentTitle
          }
          ... on CrossReferencedEvent {
            actor { __typename login ... on User { name } } createdAt
            willCloseTarget
            source {
              __typename
              ... on Issue { number title issueState: state url repository { nameWithOwner } }
              ... on PullRequest { number title state url repository { nameWithOwner } }
            }
          }
          ... on LockedEvent {
            actor { __typename login ... on User { name } } createdAt lockReason
          }
          ... on UnlockedEvent { actor { __typename login ... on User { name } } createdAt }
          ... on MilestonedEvent {
            actor { __typename login ... on User { name } } createdAt milestoneTitle
          }
          ... on DemilestonedEvent {
            actor { __typename login ... on User { name } } createdAt milestoneTitle
          }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`, rateLimitField)

// ListIssueTimeline returns a page of up to first items of the timeline of
// issue number of repo, oldest first, as GitHub orders it. Cursor and first
// work as in ListPullRequestTimeline.
func (c *Client) ListIssueTimeline(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.TimelineItem], error) {
	var data struct {
		Repository *struct {
			Issue *struct {
				Page struct {
					Nodes    []timelineNode `json:"nodes"`
					PageInfo pageInfo       `json:"pageInfo"`
				} `json:"page"`
			} `json:"issue"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number, "first": first}
	if cursor != "" {
		vars["after"] = cursor
	}
	err := c.Query(ctx, issueTimelineQuery, vars, &data)
	if err == nil && (data.Repository == nil || data.Repository.Issue == nil) {
		err = core.ErrNotFound
	}
	if err != nil {
		return core.Page[core.TimelineItem]{}, fmt.Errorf("list timeline of issue %s#%d: %w", repo, number, err)
	}
	page := data.Repository.Issue.Page
	items := make([]core.TimelineItem, 0, len(page.Nodes))
	for i := range page.Nodes {
		it, ok := page.Nodes[i].core()
		if !ok {
			slog.DebugContext(ctx, "timeline item skipped", "span", "github.timeline", "type", page.Nodes[i].Typename)
			continue
		}
		items = append(items, it)
	}
	return core.Page[core.TimelineItem]{Items: items, Next: page.PageInfo.next()}, nil
}
