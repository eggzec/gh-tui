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

// User is a GitHub account.
type User struct {
	Login string
	Name  string
}

// Label is an issue or pull request label. Color is hex without the '#'.
type Label struct {
	Name        string
	Color       string
	Description string
}

// Issue is a GitHub issue. ID is the GraphQL node ID used by mutations.
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
}

// Comment is a comment on an issue or pull request.
type Comment struct {
	ID        string
	Author    User
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
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
