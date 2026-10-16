package core

import (
	"slices"
	"strings"
	"time"
)

// PullCommit is a commit of a pull request, with how its checks stand.
type PullCommit struct {
	OID string
	// Headline is the first line of the commit message.
	Headline    string
	CommittedAt time.Time
	// Author is the name in the commit, which is not an account's: it is
	// what the committer's git was set to.
	Author string
	// Login is the account the commit's email belongs to, or empty if it
	// belongs to none.
	Login string
	// Checks is the state of the checks of the commit: CheckNone if it has
	// none.
	Checks CheckState
}

// ViewerReview is the last review the viewer submitted on a pull request.
type ViewerReview struct {
	// Commit is the SHA the review was made on, or empty if GitHub no longer
	// has the commit.
	Commit      string
	SubmittedAt time.Time
}

// PullCommits is the commits of a pull request, oldest first, with the
// viewer's last review.
type PullCommits struct {
	Commits []PullCommit
	// Capped is set when the pull request has more commits than GitHub
	// lists: the newest are missing.
	Capped bool
	// LastReview is nil if the viewer has submitted no review.
	LastReview *ViewerReview
}

// SinceReason says how SinceLastReview found its commits.
type SinceReason int

// The reasons of a [SinceResult].
const (
	// SinceNoReview is a pull request the viewer has not reviewed.
	SinceNoReview SinceReason = iota
	// SinceOnHead is a review made on the head: nothing came after it.
	SinceOnHead
	// SinceExact is a review on a commit of the list: the commits after it.
	SinceExact
	// SinceApproximate is a review on a commit the list lacks, such as one a
	// force push removed or one past the cap: the commits made after the
	// review was submitted.
	SinceApproximate
)

// SinceResult is the commits of a pull request since the viewer's last
// review.
type SinceResult struct {
	Commits []PullCommit
	Reason  SinceReason
	// Approximate is set exactly when Reason is [SinceApproximate]: the
	// commits are those dated after the review, which a clock that is off or
	// a rebase can skew.
	Approximate bool
}

// SinceLastReview returns the commits of pc that came after the viewer's last
// review, oldest first.
//
// A review on a commit of the list gives the commits after it. If that commit
// is the head, there are none. A review on a commit the list lacks, or on the
// last of a capped list, whose head is not known, gives the commits dated
// strictly after the review was submitted, and the result is approximate.
func SinceLastReview(pc PullCommits) SinceResult {
	r := pc.LastReview
	if r == nil {
		return SinceResult{Reason: SinceNoReview}
	}
	at := -1
	if r.Commit != "" {
		at = slices.IndexFunc(pc.Commits, func(c PullCommit) bool { return strings.EqualFold(c.OID, r.Commit) })
	}
	// The last commit of a capped list is not the head, so a review on it
	// says nothing of the commits that were left out.
	if at >= 0 && (!pc.Capped || at != len(pc.Commits)-1) {
		if at == len(pc.Commits)-1 {
			return SinceResult{Reason: SinceOnHead}
		}
		return SinceResult{Commits: slices.Clone(pc.Commits[at+1:]), Reason: SinceExact}
	}
	var after []PullCommit
	for _, c := range pc.Commits {
		if c.CommittedAt.After(r.SubmittedAt) {
			after = append(after, c)
		}
	}
	return SinceResult{Commits: after, Reason: SinceApproximate, Approximate: true}
}
