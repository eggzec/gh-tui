package core

import "time"

// SubjectType is the kind of thing a notification is about.
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
	URL   string
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
