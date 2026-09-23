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
	Draft          bool
	HeadRef        string
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
	// CheckRuns are the checks of the head commit.
	CheckRuns []CheckRun
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

// CheckRun is a single CI check on a commit. Status and Conclusion are
// GitHub's values in lower case, such as "completed" and "success". Commit
// statuses are reported the same way, with Status "pending" or "completed".
type CheckRun struct {
	Name       string
	Status     string
	Conclusion string
	URL        string
}
