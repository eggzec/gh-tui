package core

import "time"

// ReviewDecision is the overall review verdict on a pull request.
type ReviewDecision string

// Review decisions. ReviewNone means reviews are not required.
const (
	ReviewNone             ReviewDecision = ""
	ReviewRequired         ReviewDecision = "review_required"
	ReviewApproved         ReviewDecision = "approved"
	ReviewChangesRequested ReviewDecision = "changes_requested"
)

// ChecksState summarizes the CI checks on a commit.
type ChecksState string

// Check states. ChecksNone means the commit has no checks.
const (
	ChecksNone    ChecksState = ""
	ChecksPending ChecksState = "pending"
	ChecksSuccess ChecksState = "success"
	ChecksFailure ChecksState = "failure"
)

// PullRequest is a GitHub pull request. GitHub models pull requests as
// issues, so it embeds Issue.
type PullRequest struct {
	Issue
	Draft   bool
	HeadRef string
	// HeadSHA is the commit the head branch was at when it was read, which
	// a merge confirmed for it is pinned to.
	HeadSHA        string
	BaseRef        string
	ReviewDecision ReviewDecision
	Checks         ChecksState
	Additions      int
	Deletions      int
	ChangedFiles   int
	MergedAt       time.Time
}

// PullRequestDetail is a pull request with what the head of its detail view
// shows. The embedded PullRequest has Body set, which list results leave
// empty. Reviews and comments are read a page at a time instead, since a
// thread can be long.
type PullRequestDetail struct {
	PullRequest
	// CheckCounts counts the checks of the head commit by outcome. The
	// checks themselves are a read of their own.
	CheckCounts CheckCounts
	// FailingChecks are the first checks of the head commit that failed,
	// with why, for the Overview. ChecksTruncated says the head commit
	// has more checks than were read, which the counts cover but this list
	// and Merge.RequiredChecks may miss.
	FailingChecks   []FailingCheck
	ChecksTruncated bool
	// Merge says whether it can merge and what stands in the way.
	Merge MergeInfo
	// Reviewers are who was asked to review it and what they answered.
	Reviewers Reviewers
	// Threads summarizes its review threads.
	Threads ThreadSummary
	// BaseSHA is the commit its base branch was at when it was read.
	BaseSHA string
	// HeadRepo is the repository its head branch is in, as owner/name, or
	// empty when that is gone, such as a deleted fork; CrossRepo reports
	// whether it is another than the base's.
	HeadRepo  string
	CrossRepo bool
	// Milestone is the title of its milestone, or empty.
	Milestone string
}

// CheckCounts counts the checks of a commit, its check runs and commit
// statuses together, by outcome. Passed includes the neutral and skipped
// ones, and Failed every other that completed.
type CheckCounts struct {
	Passed, Failed, Pending int
}

// Total returns how many checks there are.
func (c CheckCounts) Total() int {
	return c.Passed + c.Failed + c.Pending
}

// MergeMethod is how a pull request is merged into its base branch.
type MergeMethod string

// Merge methods.
const (
	MergeCommit MergeMethod = "merge"
	MergeSquash MergeMethod = "squash"
	MergeRebase MergeMethod = "rebase"
)

// ReviewState is the state of a single review.
type ReviewState string

// Review states.
const (
	ReviewStatePending          ReviewState = "pending"
	ReviewStateCommented        ReviewState = "commented"
	ReviewStateApproved         ReviewState = "approved"
	ReviewStateChangesRequested ReviewState = "changes_requested"
	ReviewStateDismissed        ReviewState = "dismissed"
)

// Review is a pull request review.
type Review struct {
	ID          string
	Author      User
	State       ReviewState
	Body        string
	SubmittedAt time.Time
}

// MaxPullFiles is the most files GitHub lists of one pull request. Files
// past it are left out, and the last page of the list is Truncated.
const MaxPullFiles = 3000
