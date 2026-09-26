package core

// NumberKind says what a number of a repository names. GitHub numbers the
// issues and pull requests of a repository in one sequence, so a number
// alone doesn't tell which one it is. The zero value means unknown.
type NumberKind string

// Kinds of numbers.
const (
	KindIssue NumberKind = "issue"
	KindPull  NumberKind = "pull"
)

// Known reports whether k is an issue or a pull request.
func (k NumberKind) Known() bool {
	return k == KindIssue || k == KindPull
}
