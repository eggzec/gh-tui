package core

import "errors"

// SearchKind is what a search looks for, or what a search hit is.
type SearchKind string

// Search kinds. SearchAll looks for repositories, issues and pull requests;
// a hit is never SearchAll. SearchCode looks for files, whose hits are
// CodeHits.
const (
	SearchAll    SearchKind = ""
	SearchRepos  SearchKind = "repos"
	SearchIssues SearchKind = "issues"
	SearchPulls  SearchKind = "pulls"
	SearchCode   SearchKind = "code"
)

// SearchHit is one search result: a repository, or an issue or pull
// request. Kind says which of Repo and Issue is set. For a pull request,
// Issue is its issue part, and Issue.Repo names its repository in either
// case.
type SearchHit struct {
	Kind  SearchKind
	Repo  Repo
	Issue Issue
	// Draft reports a draft pull request.
	Draft bool
}

// SearchPage is a page of search results of one kind with the number of
// results in all, which GitHub counts up to a limit of its own.
type SearchPage[T any] struct {
	Page[T]
	Total int
	// Incomplete reports that GitHub ran out of time, so Total and the
	// results may miss some matches.
	Incomplete bool
}

// CodeHit is a file that matches a code search, at the commit GitHub
// indexed.
type CodeHit struct {
	Repo RepoRef
	Path string
	// SHA is the blob's SHA.
	SHA string
	// URL is the file's page on GitHub.
	URL       string
	Fragments []Fragment
}

// Fragment is an excerpt of a file around matches of a code search.
// Matches holds the start and end of each match as byte offsets into Text,
// so Text[m[0]:m[1]] is the matched text.
type Fragment struct {
	Text    string
	Matches [][2]int
}

// ErrInvalidQuery is matched by an *InvalidQueryError.
var ErrInvalidQuery = errors.New("invalid search query")

// InvalidQueryError reports a search query that GitHub refused, such as
// one with a malformed qualifier. Reason is GitHub's explanation.
type InvalidQueryError struct {
	Reason string
}

func (e *InvalidQueryError) Error() string {
	if e.Reason == "" {
		return "invalid search query"
	}
	return "invalid search query: " + e.Reason
}

// Is reports whether target is ErrInvalidQuery.
func (e *InvalidQueryError) Is(target error) bool {
	return target == ErrInvalidQuery
}
