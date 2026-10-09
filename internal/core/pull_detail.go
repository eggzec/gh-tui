package core

import (
	"slices"
	"time"
)

// Mergeable says whether GitHub can merge a pull request's head into its
// base without conflicts, as it works it out.
type Mergeable string

// Mergeable values. MergeableUnknown means GitHub is still working it out,
// which it does in the background after a push: read the pull request
// again a moment later. The zero value means the read didn't say.
const (
	MergeableYes         Mergeable = "mergeable"
	MergeableConflicting Mergeable = "conflicting"
	MergeableUnknown     Mergeable = "unknown"
)

// MergeStatus is GitHub's verdict on a pull request's merge, which folds
// conflicts, the branch's rules, the checks and the reviews into one. The
// zero value means the read didn't say.
type MergeStatus string

// Merge statuses, as GitHub's MergeStateStatus names them.
const (
	// MergeClean: mergeable, and the required checks pass.
	MergeClean MergeStatus = "clean"
	// MergeBlocked: the branch's rules, such as required checks or
	// reviews, keep it from merging.
	MergeBlocked MergeStatus = "blocked"
	// MergeBehind: the head is behind its base, and the rules need it up
	// to date.
	MergeBehind MergeStatus = "behind"
	// MergeDirty: the merge commit can't be created: conflicts.
	MergeDirty MergeStatus = "dirty"
	// MergeDraft: a draft can't be merged.
	MergeDraft MergeStatus = "draft"
	// MergeHasHooks: mergeable, with passing status and pre-receive hooks.
	MergeHasHooks MergeStatus = "has_hooks"
	// MergeUnstable: mergeable, with a failing or pending check that the
	// rules don't require.
	MergeUnstable MergeStatus = "unstable"
	// MergeUnknown: GitHub is still working it out.
	MergeUnknown MergeStatus = "unknown"
)

// MergeInfo is what the Overview of a pull request needs to say whether it
// can merge, and what stands in the way. It is read with the pull request
// and may be older than its checks and reviews, which are read apart.
type MergeInfo struct {
	Mergeable Mergeable
	Status    MergeStatus
	// Methods are the merge methods the repository allows, in the order
	// merge, squash, rebase.
	Methods []MergeMethod
	// AutoMerge is set while auto-merge is enabled: the pull request
	// merges once its rules are met.
	AutoMerge *AutoMerge
	// CanAutoMerge and CanDisableAutoMerge say what the viewer may do
	// about auto-merge. The repository must allow it (RepoAutoMerge) and
	// the pull request must not be ready to merge.
	CanAutoMerge, CanDisableAutoMerge bool
	// RepoAutoMerge reports whether the repository allows auto-merge.
	RepoAutoMerge bool
	// CanMergeAsAdmin reports whether the viewer may merge past the
	// rules.
	CanMergeAsAdmin bool
	// Queue is the merge queue of the base branch.
	Queue MergeQueue
	// Rules are the branch rules the viewer's token may read.
	Rules MergeRules
	// RequiredChecks counts the checks the rules require by outcome, of
	// the first checks read: see PullRequestDetail.ChecksTruncated.
	RequiredChecks CheckCounts
}

// Allows reports whether the repository allows merging with m.
func (m MergeInfo) Allows(method MergeMethod) bool {
	return slices.Contains(m.Methods, method)
}

// AutoMerge is an enabled auto-merge.
type AutoMerge struct {
	// Method is how it merges, or empty if the read didn't say.
	Method    MergeMethod
	EnabledBy string
	EnabledAt time.Time
}

// MergeQueue says whether the base branch of a pull request has a merge
// queue, and where the pull request is in it.
type MergeQueue struct {
	// Enabled reports whether the base branch has a merge queue: merging
	// then adds the pull request to it.
	Enabled bool
	// Queued reports whether the pull request is in the queue, and
	// Position is its place there, from 1, and State its state, such as
	// "queued" or "awaiting_checks".
	Queued   bool
	Position int
	State    string
	// EnqueuedAt is when it joined the queue.
	EnqueuedAt time.Time
}

// MergeRules are the rules of the base branch that a read could see: its
// branch protection and the rulesets that apply to it. Known is set when
// the read found any: a branch without rules, and one whose rules the
// token may not read, look alike, so that an unset Known says only that
// none are known, and the merge status may still say the merge is
// blocked.
type MergeRules struct {
	Known bool
	// Approvals is how many approving reviews are required.
	Approvals int
	// Checks are the names of the checks that must pass.
	Checks []string
	// CodeOwners, Conversations, LinearHistory and Signatures report
	// whether a review of a code owner, resolved conversations, a linear
	// history and signed commits are required.
	CodeOwners, Conversations, LinearHistory, Signatures bool
}

// ReviewRequest is a review asked of a person or a team.
type ReviewRequest struct {
	// User is the person asked, or zero for a team.
	User User
	// Team is the team asked, as org/slug, or empty for a person.
	Team string
	// Unknown reports that neither is set: the reviewer is one the token
	// may not see, such as a private team, or one of a kind the read
	// doesn't know.
	Unknown bool
	// CodeOwner reports whether they were asked as a code owner.
	CodeOwner bool
}

// Verdict is the latest review of one person that took a side.
type Verdict struct {
	Author      User
	State       ReviewState
	SubmittedAt time.Time
}

// Reviewers is who was asked to review a pull request and what each
// answered. Lists hold the first few of each, and the totals say how many
// there are.
type Reviewers struct {
	Requested      []ReviewRequest
	RequestedTotal int
	// Verdicts are the latest review of each reviewer that approved,
	// requested changes or was dismissed: a review that only commented
	// leaves the earlier verdict standing.
	Verdicts      []Verdict
	VerdictsTotal int
	// Viewer is the state of the viewer's latest review: ReviewStatePending
	// means a review they started, on the web, that is not yet submitted.
	// Empty if they have none.
	Viewer ReviewState
}

// ReviewThread is a conversation on a line or file of a pull request's
// diff, with its first comment.
type ReviewThread struct {
	ID       string
	Path     string
	Line     int
	Resolved bool
	// Outdated reports whether the code it is about has changed since.
	Outdated bool
	// Comments counts the comments of the thread.
	Comments int
	// Author, At and Excerpt are of its first comment: the excerpt is the
	// start of its text on one line.
	Author  User
	At      time.Time
	Excerpt string
}

// ThreadSummary counts the review threads of a pull request and lists the
// first of them. GitHub can't count them by state, so the counts cover the
// threads listed: Truncated says there are more, which the first page left
// out.
type ThreadSummary struct {
	// Total is how many threads the pull request has.
	Total                          int
	Unresolved, Resolved, Outdated int
	Threads                        []ReviewThread
	Truncated                      bool
}

// FailingCheck is a check that failed on the head commit of a pull
// request, with the reason to show beside its name.
type FailingCheck struct {
	Name string
	// Run reports whether it is a check run, whose ID leads to its job and
	// log, rather than a commit status.
	Run bool
	ID  int64
	// Required reports whether the pull request needs it to pass.
	Required bool
	// Reason is its title, or for a commit status its description, on one
	// line; empty if it has none.
	Reason string
}
