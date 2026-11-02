package core

import "time"

// State is the lifecycle state of an issue or pull request.
type State string

// States of issues and pull requests.
const (
	StateOpen   State = "open"
	StateClosed State = "closed"
	StateMerged State = "merged"
)

// StateReason says why an issue was closed, or that it was reopened. It is
// empty when GitHub doesn't say, as for pull requests.
type StateReason string

// State reasons. An issue closed as not planned or as a duplicate is shown
// apart from one that was completed.
const (
	ReasonCompleted  StateReason = "completed"
	ReasonNotPlanned StateReason = "not_planned"
	ReasonDuplicate  StateReason = "duplicate"
	ReasonReopened   StateReason = "reopened"
)

// StateFilter selects issues or pull requests by state when listing them.
type StateFilter string

// State filters. The zero value lists open ones, as GitHub does.
const (
	FilterOpen   StateFilter = "open"
	FilterClosed StateFilter = "closed"
	FilterAll    StateFilter = "all"
)

// User is a GitHub account. Bot reports an app, such as dependabot, which
// has no page of its own.
type User struct {
	Login string
	Name  string
	Bot   bool
}

// Label is an issue or pull request label. Color is hex without the '#'.
type Label struct {
	Name        string
	Color       string
	Description string
}

// Issue is a GitHub issue. ID is the GraphQL node ID used by mutations.
// A Locked issue takes comments only from collaborators; LockReason says
// why, such as "too_heated", or is empty.
type Issue struct {
	ID     string
	Repo   RepoRef
	Number int
	Title  string
	Body   string
	State  State
	// Reason says why a closed issue was closed.
	Reason    StateReason
	Author    User
	Labels    []Label
	Assignees []User
	Comments  int
	CreatedAt time.Time
	UpdatedAt time.Time
	URL       string

	Locked     bool
	LockReason string
	// Caps are what the viewer may do to it, when the read said.
	Caps ItemCaps
}

// Comment is a comment on an issue or pull request.
type Comment struct {
	ID     string
	Author User
	// AvatarURL is the address of the author's avatar as GitHub gave it,
	// or empty for a deleted account.
	AvatarURL string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
	// Reactions counts the reactions of the comment, only the kinds that
	// someone gave, in GitHub's order.
	Reactions []Reaction
}

// Reaction is how many people gave one kind of reaction to a comment.
type Reaction struct {
	// Content is the kind, as GitHub's ReactionContent names it: THUMBS_UP,
	// THUMBS_DOWN, LAUGH, HOORAY, CONFUSED, HEART, ROCKET or EYES.
	Content string
	Count   int
}

// Milestone is a milestone of a repository, which issues and pull requests
// can be filed under.
type Milestone struct {
	Number int
	Title  string
	State  State
	// DueOn is zero for a milestone without a due date.
	DueOn time.Time
}
