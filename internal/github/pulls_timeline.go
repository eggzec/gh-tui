package github

import (
	"context"
	"fmt"

	"github.com/eggzec/gh-tui/internal/core"
)

// pullTimelineQuery reads a page of the timeline of a pull request, oldest
// first, of the item types that core.TimelineKind lists. GitHub's union has
// dozens more, such as subscriptions and project moves, which the
// conversation has no use for.
//
// A field that two members of the union select with different types is
// aliased (the commit of a pushed commit, the state of an issue), since
// GraphQL refuses to merge them.
var pullTimelineQuery = fmt.Sprintf(`query PullTimeline($owner: String!, $name: String!, $number: Int!, $first: Int!, $after: String) {
  %s
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      page: timelineItems(first: $first, after: $after, itemTypes: [
        ISSUE_COMMENT PULL_REQUEST_REVIEW PULL_REQUEST_COMMIT HEAD_REF_FORCE_PUSHED_EVENT
        LABELED_EVENT UNLABELED_EVENT ASSIGNED_EVENT UNASSIGNED_EVENT
        REVIEW_REQUESTED_EVENT REVIEW_REQUEST_REMOVED_EVENT
        READY_FOR_REVIEW_EVENT CONVERT_TO_DRAFT_EVENT
        MERGED_EVENT CLOSED_EVENT REOPENED_EVENT RENAMED_TITLE_EVENT BASE_REF_CHANGED_EVENT
        CROSS_REFERENCED_EVENT HEAD_REF_DELETED_EVENT REVIEW_DISMISSED_EVENT
        LOCKED_EVENT UNLOCKED_EVENT MILESTONED_EVENT DEMILESTONED_EVENT
      ]) {
        nodes {
          __typename
          ... on IssueComment {
            id author { __typename login avatarUrl ... on User { name } }
            body createdAt updatedAt url viewerCanUpdate viewerCanDelete
            reactionGroups { content reactors { totalCount } }
          }
          ... on PullRequestReview {
            id author { __typename login avatarUrl ... on User { name } }
            state body createdAt submittedAt url
            reactionGroups { content reactors { totalCount } }
            commit { oid abbreviatedOid }
          }
          ... on PullRequestCommit {
            url
            pullCommit: commit {
              oid abbreviatedOid messageHeadline committedDate
              author { name user { login name } }
            }
          }
          ... on HeadRefForcePushedEvent {
            actor { __typename login ... on User { name } } createdAt
            beforeCommit { oid } afterCommit { oid }
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
          ... on ReviewRequestedEvent {
            actor { __typename login ... on User { name } } createdAt
            requestedReviewer { __typename ... on Actor { login } ... on User { name } ... on Team { combinedSlug } }
          }
          ... on ReviewRequestRemovedEvent {
            actor { __typename login ... on User { name } } createdAt
            requestedReviewer { __typename ... on Actor { login } ... on User { name } ... on Team { combinedSlug } }
          }
          ... on ReadyForReviewEvent { actor { __typename login ... on User { name } } createdAt }
          ... on ConvertToDraftEvent { actor { __typename login ... on User { name } } createdAt }
          ... on MergedEvent {
            actor { __typename login ... on User { name } } createdAt
            mergeRefName commit { oid abbreviatedOid }
          }
          ... on ClosedEvent { actor { __typename login ... on User { name } } createdAt }
          ... on ReopenedEvent { actor { __typename login ... on User { name } } createdAt }
          ... on RenamedTitleEvent {
            actor { __typename login ... on User { name } } createdAt
            previousTitle currentTitle
          }
          ... on BaseRefChangedEvent {
            actor { __typename login ... on User { name } } createdAt
            previousRefName currentRefName
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
          ... on HeadRefDeletedEvent {
            actor { __typename login ... on User { name } } createdAt headRefName
          }
          ... on ReviewDismissedEvent {
            actor { __typename login ... on User { name } } createdAt dismissalMessage
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

// ListPullRequestTimeline returns a page of up to first items of the
// timeline of pull request number of repo, oldest first, as GitHub orders
// it. Cursor and first work as in ListPullRequests. Items of kinds the
// client doesn't know are left out, so a page may hold fewer than first
// while another follows.
func (c *Client) ListPullRequestTimeline(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.TimelineItem], error) {
	var data struct {
		Repository *struct {
			PullRequest *struct {
				Page struct {
					Nodes    []timelineNode `json:"nodes"`
					PageInfo pageInfo       `json:"pageInfo"`
				} `json:"page"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number, "first": first}
	if cursor != "" {
		vars["after"] = cursor
	}
	err := c.Query(ctx, pullTimelineQuery, vars, &data)
	found := data.Repository != nil && data.Repository.PullRequest != nil
	if err := timelineErr(err, found, "pull request", repo, number); err != nil {
		return core.Page[core.TimelineItem]{}, err
	}
	page := data.Repository.PullRequest.Page
	return timelineItems(ctx, page.Nodes, page.PageInfo), nil
}
