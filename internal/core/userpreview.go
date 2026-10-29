package core

import "time"

// UserProfile is what the search preview of a user or an organization reads
// beyond the hit. For an organization, Bio is its description and Followers
// its followers, as GET /users/{login} gives them.
type UserProfile struct {
	Login     string
	Kind      OwnerKind
	Name      string
	Bio       string
	Followers int
	CreatedAt time.Time
}

// MaxCommitPatch is the most bytes of a commit's patch that the search
// preview reads. A larger patch is not read.
const MaxCommitPatch = 2 << 20

// CommitPreview is what the search preview of a commit shows: the commit with
// the lines each file changed, and its whole patch unless that is over
// MaxCommitPatch, which PatchTooLarge reports. The files of Detail carry no
// patches of their own.
type CommitPreview struct {
	Detail        CommitDetail
	Patch         string
	PatchTooLarge bool
}
