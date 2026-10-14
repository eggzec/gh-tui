package core

// PendingReview is the viewer's own review of a pull request that is not
// submitted yet, such as one started on the web. GitHub allows one per pull
// request and shows it to its author only.
type PendingReview struct {
	ID string
	// Commit is the commit the review is on, which new comments added to it
	// land on, whatever the pull request's head is.
	Commit string
	// Body is the summary written so far.
	Body string
	// Comments are all of its comments, in the order GitHub gives them.
	Comments []PendingComment
}

// PendingComment is a comment of a pending review. Whether it starts a thread
// or answers one is told by ReplyTo.
type PendingComment struct {
	ID   string
	Path string
	// Line is where the comment ends in the head's file, and StartLine where
	// a comment on several lines starts, 0 for one line. Both are 0 once the
	// code changed, or on a whole file: see OriginalLine.
	Line, StartLine int
	// OriginalLine and OriginalStartLine are the same as the comment was made.
	OriginalLine, OriginalStartLine int
	// ReplyTo is the ID of the comment that this one answers, empty if it
	// starts a thread.
	ReplyTo string
	Body    string
	// Outdated reports that the code it is about has changed since.
	Outdated bool
	// File reports a comment on the whole file rather than on lines.
	File bool
}
