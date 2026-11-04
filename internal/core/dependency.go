package core

// Dependency is an issue that blocks, or is blocked by, another.
type Dependency struct {
	Repo   RepoRef
	Number int
	Title  string
	State  State
	Reason StateReason
	URL    string
	// IsPullRequest is set for a pull request.
	IsPullRequest bool
}
