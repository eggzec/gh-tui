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
	ID        string
	Repo      RepoRef
	Number    int
	Title     string
	Body      string
	State     State
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
