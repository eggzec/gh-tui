package core

// VerifiedKind says what a candidate for an automatic link turned out to be.
type VerifiedKind string

// The kinds of thing a candidate may name.
const (
	VerifiedIssue  VerifiedKind = "issue"
	VerifiedPull   VerifiedKind = "pull"
	VerifiedUser   VerifiedKind = "user"
	VerifiedOrg    VerifiedKind = "org"
	VerifiedCommit VerifiedKind = "commit"
)

// Verified is GitHub's answer to whether something a text names exists.
type Verified struct {
	Exists bool
	// URL is the address GitHub gives the thing, empty if it doesn't exist.
	URL  string
	Kind VerifiedKind
}
