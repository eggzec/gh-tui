package core

import "time"

// LinkedIssue is an issue that a pull request closes, with the issues that
// block it. Blockers go one level deep: a blocker's own blockers are not read.
type LinkedIssue struct {
	Issue Reference
	// Blockers are the first issues that block it, and BlockersTotal how many
	// there are.
	Blockers      []Reference
	BlockersTotal int
}

// PullLinks is what a pull request closes, and the other pull requests that
// close the same issues.
type PullLinks struct {
	// Closes are the first issues it closes, and ClosesTotal how many it does.
	Closes      []LinkedIssue
	ClosesTotal int
	// Linked are the other pull requests that close those issues, without
	// duplicates and this one left out. Those of its repository come first,
	// then those of other repositories by name, each the lowest number first,
	// and last a row with Problem set for each that GitHub refused.
	Linked []Reference
	// LinkedCapped reports that an issue has more pull requests that close
	// it than were read.
	LinkedCapped bool
	// BlockersSupported is false on a server without issue dependencies,
	// which the blockers are left empty for.
	BlockersSupported bool
}

// PreviewComment is a comment of an issue, as its preview shows it.
type PreviewComment struct {
	Author    string
	Body      string
	CreatedAt time.Time
}

// PreviewReview is the latest opinionated review of one reviewer of a pull
// request, as its preview shows it.
type PreviewReview struct {
	Author string
	State  ReviewState
}

// IssuePreview is the short read of an issue that its preview shows.
type IssuePreview struct {
	Reference
	Author    string
	CreatedAt time.Time
	Labels    []Label
	// Blockers are the first issues that block it, and BlockersTotal how many
	// there are. BlockersSupported is false on a server without issue
	// dependencies.
	Blockers          []Reference
	BlockersTotal     int
	BlockersSupported bool
	Body              string
	// Comments are the last few comments, the oldest first, of CommentsTotal.
	Comments      []PreviewComment
	CommentsTotal int
}

// PullPreview is the short read of a pull request that its preview shows.
type PullPreview struct {
	Reference
	Author                             string
	Head, Base                         string
	Additions, Deletions, ChangedFiles int
	ReviewDecision                     ReviewDecision
	// Reviews are the latest opinionated review of each reviewer.
	Reviews []PreviewReview
	Labels  []Label
	// Checks counts the checks of the head commit, and is zero for a commit
	// with none.
	Checks CheckCounts
	Body   string
}
