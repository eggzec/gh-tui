package core

import "time"

// SubjectType is the kind of thing a notification is about. Types without a
// constant here, such as CheckSuite, keep the name GitHub gives them.
type SubjectType string

// Subject types.
const (
	SubjectIssue       SubjectType = "Issue"
	SubjectPullRequest SubjectType = "PullRequest"
	SubjectRelease     SubjectType = "Release"
	SubjectDiscussion  SubjectType = "Discussion"
	SubjectCommit      SubjectType = "Commit"
)

// Subject is what a notification points at.
type Subject struct {
	Title string
	Type  SubjectType
	// URL is the API URL of the subject. It is empty for subjects that
	// have none, such as discussions.
	URL string
	// WebURL is the page to open in a browser. When GitHub doesn't say
	// which page, as for discussions and releases, it is the closest page
	// of the repository.
	WebURL string
}

// Notification is an entry in the user's inbox.
type Notification struct {
	ID        string
	Repo      RepoRef
	Subject   Subject
	Reason    string
	Unread    bool
	UpdatedAt time.Time
}

// NotificationFilter selects the threads of an inbox listing. The zero value
// is the default inbox: unread threads the user is subscribed to.
type NotificationFilter struct {
	// All includes threads that were already read.
	All bool
	// Participating keeps only threads the user takes part in, for example
	// by being mentioned, assigned or asked for a review.
	Participating bool
}
