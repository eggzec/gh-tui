package core

import "fmt"

// NoNumberError reports that a repository has no issue or pull request of
// a number that the account may see: GitHub answered 404, which matches
// ErrNotFound, or 410, for a deleted issue or a repository with its issues
// turned off. It unwraps to what GitHub answered.
type NoNumberError struct {
	Repo   RepoRef
	Number int
	Err    error
}

func (e *NoNumberError) Error() string {
	return fmt.Sprintf("no issue or pull request %s#%d", e.Repo, e.Number)
}

// Unwrap returns what GitHub answered.
func (e *NoNumberError) Unwrap() error {
	return e.Err
}
