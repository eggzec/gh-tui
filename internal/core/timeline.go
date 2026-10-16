package core

import "time"

// TimelineKind says what a TimelineItem is, and so which of its fields hold
// something.
type TimelineKind string

// The kinds of items on the timeline of a pull request.
const (
	// TimelineComment is a comment of the conversation. ID, Body, URL,
	// UpdatedAt, CanUpdate and CanDelete say about it.
	TimelineComment TimelineKind = "comment"
	// TimelineReview is a submitted review. ID, Review, Body, URL and
	// Commit (the commit it was made on) say about it.
	TimelineReview TimelineKind = "review"
	// TimelinePushed is a commit that was added to the branch. Commit says
	// which, and Actor is the author when GitHub knows their account.
	TimelinePushed TimelineKind = "commit"
	// TimelineForcePushed is a force push of the branch: From and To are the
	// commits it pointed at before and after.
	TimelineForcePushed TimelineKind = "force_pushed"
	// TimelineLabeled and TimelineUnlabeled add and remove Label.
	TimelineLabeled   TimelineKind = "labeled"
	TimelineUnlabeled TimelineKind = "unlabeled"
	// TimelineAssigned and TimelineUnassigned add and remove Subject.
	TimelineAssigned   TimelineKind = "assigned"
	TimelineUnassigned TimelineKind = "unassigned"
	// TimelineReviewRequested and TimelineReviewRequestRemoved ask a review of
	// Subject, a person or a team, and take it back.
	TimelineReviewRequested      TimelineKind = "review_requested"
	TimelineReviewRequestRemoved TimelineKind = "review_request_removed"
	// TimelineReadyForReview and TimelineConvertedToDraft mark a pull request
	// ready, and a draft again.
	TimelineReadyForReview   TimelineKind = "ready_for_review"
	TimelineConvertedToDraft TimelineKind = "converted_to_draft"
	// TimelineMerged is the merge. Commit is the merge commit and Ref the
	// branch it went into.
	TimelineMerged   TimelineKind = "merged"
	TimelineClosed   TimelineKind = "closed"
	TimelineReopened TimelineKind = "reopened"
	// TimelineRenamed changes the title from From to To.
	TimelineRenamed TimelineKind = "renamed"
	// TimelineBaseChanged moves the base branch from From to To.
	TimelineBaseChanged TimelineKind = "base_changed"
	// TimelineCrossReferenced is another issue or pull request, Source, that
	// mentions this one.
	TimelineCrossReferenced TimelineKind = "cross_referenced"
	// TimelineHeadDeleted deletes the head branch Ref.
	TimelineHeadDeleted TimelineKind = "head_deleted"
	// TimelineReviewDismissed dismisses a review. Body is the reason.
	TimelineReviewDismissed TimelineKind = "review_dismissed"
	// TimelineLocked and TimelineUnlocked lock the conversation, for Ref.
	TimelineLocked   TimelineKind = "locked"
	TimelineUnlocked TimelineKind = "unlocked"
	// TimelineMilestoned and TimelineDemilestoned add and remove the
	// milestone titled Ref.
	TimelineMilestoned   TimelineKind = "milestoned"
	TimelineDemilestoned TimelineKind = "demilestoned"
)

// TimelineItem is one entry of the timeline of a pull request: something
// that was said or done. Only the fields that its Kind names hold a value.
type TimelineItem struct {
	Kind TimelineKind
	// ID is the node ID of a comment or review.
	ID string
	// Actor is who did it, empty for a deleted account. For a pushed commit it
	// is the commit's author, who may not be the one who pushed it.
	Actor User
	// AvatarURL is the address of the actor's avatar for a comment or a
	// review, as GitHub gave it, or empty.
	AvatarURL string
	// At is when it happened: when a comment or a review was made, or a
	// commit was committed. A pushed commit uses its commit date, not the time
	// of the push, so At doesn't follow the order of the timeline: callers keep
	// the order GitHub gave and never sort by At.
	At        time.Time
	UpdatedAt time.Time
	Body      string
	URL       string
	// CanUpdate and CanDelete are what the viewer may do to a comment.
	CanUpdate bool
	CanDelete bool
	Review    ReviewState
	Commit    TimelineCommit
	// From and To are the two sides of a change: SHAs of a force push, the
	// titles of a rename, the branches of a base change.
	From, To string
	// Ref names a branch, a lock reason or a milestone, by Kind.
	Ref   string
	Label Label
	// Subject is the person or the team that an assignment or a review
	// request is about.
	Subject TimelineSubject
	Source  TimelineSource
}

// TimelineCommit is the commit of a timeline item.
type TimelineCommit struct {
	// OID is the full SHA and Short its abbreviation.
	OID, Short string
	// Headline is the first line of the message.
	Headline string
	// Author is the commit's author, with the login if the account is known.
	Author User
}

// TimelineSubject is who an assignment or a review request is about.
type TimelineSubject struct {
	User User
	// Team is the team's name with its organization, such as "eggzec/core",
	// when the subject is a team, and then User is empty.
	Team string
}

// TimelineSource is the issue or pull request that mentions another.
type TimelineSource struct {
	Number int
	Title  string
	State  State
	URL    string
	// Repo is the repository it is in, as "owner/name".
	Repo          string
	IsPullRequest bool
	// WillClose is whether merging the source closes the one it mentions.
	WillClose bool
}
