package core

import "time"

// SubjectType is the kind of thing a notification is about. Types without a
// constant here, such as RepositoryVulnerabilityAlert, keep the name GitHub
// gives them.
type SubjectType string

// Subject types.
const (
	SubjectIssue       SubjectType = "Issue"
	SubjectPullRequest SubjectType = "PullRequest"
	SubjectRelease     SubjectType = "Release"
	SubjectDiscussion  SubjectType = "Discussion"
	SubjectCommit      SubjectType = "Commit"
	// SubjectCheckSuite is a workflow run, whose subject URL names the check
	// suite.
	SubjectCheckSuite SubjectType = "CheckSuite"
)

// Subject is what a notification points at.
type Subject struct {
	Title string
	Type  SubjectType
	// URL is the API URL of the subject. It is empty for subjects that
	// have none, such as discussions.
	URL string
	// WebURL is the page to open in a browser. When GitHub doesn't say
	// which page, as for discussions, it is the closest page of the
	// repository. Releases and check suites start there too, until Resolved.
	WebURL string
	// Number is that of the issue, pull request or discussion, or 0 when
	// URL doesn't say.
	Number int
	// SHA is the commit of a Commit subject.
	SHA string
	// ReleaseID is the ID the API names a Release subject by.
	ReleaseID int64
	// CheckSuiteID is the ID the API names a CheckSuite subject by.
	CheckSuiteID int64
	// Resolved is set when WebURL was looked up and names the subject's own
	// page: the release, or the run of the check suite.
	Resolved bool
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
	// Since is a window of time, such as 1d, 1w or 1m, that keeps only the
	// threads updated within it. It is a token, not a time, so that what
	// keys a cached page stays the same as the clock moves; SinceTime turns
	// it into a time when the request is made. Empty, or a token SinceTime
	// doesn't know, keeps every thread.
	Since string
}

// SinceTime returns when the window of Since began, counted back from now,
// or the zero time when the filter has no window.
func (f NotificationFilter) SinceTime(now time.Time) time.Time {
	switch f.Since {
	case "1d":
		return now.AddDate(0, 0, -1)
	case "1w":
		return now.AddDate(0, 0, -7)
	case "1m":
		return now.AddDate(0, -1, 0)
	}
	return time.Time{}
}
