package core

import "time"

// Side is the side of a diff that a review thread was placed on.
type Side string

// Sides of a diff.
const (
	// SideRight is the new file: added and unchanged lines. It is where
	// most threads are, and the zero value reads as it.
	SideRight Side = "right"
	// SideLeft is the old file: removed lines.
	SideLeft Side = "left"
)

// Thread is a conversation of review comments on a line, a range of lines or
// a whole file of a pull request's diff, with every comment that was read.
// ReviewThread is the summary of the Overview; this is what the diff and the
// conversation show.
type Thread struct {
	ID   string
	Path string
	// Line is where the thread ends in the file of Side (the head's for the
	// right side, the base's for the left), and StartLine where a thread on
	// several lines starts, which is 0 for one
	// on a single line. Both are 0 for a thread that is outdated, or on a
	// whole file: see OriginalLine.
	Line, StartLine int
	// OriginalLine and OriginalStartLine are the same as the thread was
	// made, in the commit it was made on, and stay when the code changes.
	OriginalLine, OriginalStartLine int
	// Side is the side of the diff the thread ends on, and StartSide the
	// side it starts on, which is that of Side for a thread on one line.
	Side, StartSide Side
	// File reports a thread on the whole file rather than on lines.
	File bool

	Resolved bool
	// Outdated reports whether the code it is about has changed since.
	Outdated bool
	// ResolvedBy is who resolved it, if it is, and GitHub says who.
	ResolvedBy User
	// ViewerCanResolve and ViewerCanUnresolve are what the viewer may do to
	// the thread.
	ViewerCanResolve, ViewerCanUnresolve bool

	// Comments are the comments read, oldest first. TotalComments counts all
	// of them, and MoreComments reports that it is more than were read: the
	// rest is on GitHub.
	Comments      []ThreadComment
	TotalComments int
	MoreComments  bool
}

// ThreadComment is one comment of a review thread.
type ThreadComment struct {
	ID     string
	Author User
	At     time.Time
	// Body is the comment's markdown.
	Body string
	URL  string
	// ReplyTo is the ID of the comment that this one answers, empty for the
	// first of a thread.
	ReplyTo string
	// EditedAt is when the comment was last edited, zero if it wasn't.
	EditedAt time.Time
}

// ThreadPage is one page of the review threads of a pull request. An empty
// Next means there are no more pages.
type ThreadPage struct {
	Threads []Thread
	// Total is how many threads the pull request has, of every page.
	Total int
	Next  string
	// Offline and Limited are as for Page.
	Offline, Limited bool
}

// Last reports whether this is the final page.
func (p ThreadPage) Last() bool {
	return p.Next == ""
}
