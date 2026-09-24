package core

// SearchKind is what a search looks for, or what a search hit is.
type SearchKind string

// Search kinds. SearchAll looks for every kind; a hit is never SearchAll.
const (
	SearchAll    SearchKind = ""
	SearchRepos  SearchKind = "repos"
	SearchIssues SearchKind = "issues"
	SearchPulls  SearchKind = "pulls"
)

// SearchHit is one search result: a repository, or an issue or pull
// request. Kind says which of Repo and Issue is set. For a pull request,
// Issue is its issue part, and Issue.Repo names its repository in either
// case.
type SearchHit struct {
	Kind  SearchKind
	Repo  Repo
	Issue Issue
}
