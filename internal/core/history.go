package core

import (
	"time"
)

// Branch is a branch of a repository and the commit it points at.
// Whether it is the default branch is Repo.DefaultBranch's to say.
type Branch struct {
	Name      string
	SHA       string
	Protected bool
}

// Commit is a commit as a history lists it, with everything its row or
// its header may show. Parents are the SHAs of its parents, first parent
// first, which is enough to draw the graph. TreeSHA names its root tree,
// which the files service lists.
//
// Message is the message as git recorded it. Subject, Body and Trailers
// are SplitMessage of it.
type Commit struct {
	SHA          string
	TreeSHA      string
	Parents      []string
	Message      string
	Subject      string
	Body         string
	Trailers     []Trailer
	Author       Signature
	Committer    Signature
	Verification Verification
	URL          string
}

// Merge reports whether the commit has more than one parent.
func (c Commit) Merge() bool {
	return len(c.Parents) > 1
}

// Signature is who authored or committed a commit, and when, as git
// recorded it. Login is the GitHub account whose email it is, if any.
type Signature struct {
	Name  string
	Email string
	Login string
	Date  time.Time
}

// Verification is what GitHub found checking a commit's signature. Signed
// reports that the commit has a signature at all; Reason says why it is
// or isn't Verified, such as "valid", "unsigned" or "unknown_key", as
// GitHub names it. GitHub's REST API doesn't say who signed.
type Verification struct {
	Verified   bool
	Signed     bool
	Reason     string
	VerifiedAt time.Time
}

// MaxCommitFiles is the most files GitHub lists of one commit. Files
// past it are left out, and CommitDetail.FilesTruncated is set.
const MaxCommitFiles = 3000

// CommitDetail is a commit with what it changed against its first parent.
// Files holds the first page of the changed files; FilesNext is the cursor
// of the next page, or empty if Files holds them all.
type CommitDetail struct {
	Commit
	Stats     CommitStats
	Files     []CommitFile
	FilesNext string
	// FilesTruncated reports that the commit changed more files than
	// GitHub lists, MaxCommitFiles, so the last page misses some.
	FilesTruncated bool
}

// CommitStats counts the lines a commit changed. Total is Additions plus
// Deletions.
type CommitStats struct {
	Additions int
	Deletions int
	Total     int
}

// FileStatus is how a commit changed a file, as GitHub names it.
type FileStatus string

// File statuses.
const (
	FileAdded     FileStatus = "added"
	FileRemoved   FileStatus = "removed"
	FileModified  FileStatus = "modified"
	FileRenamed   FileStatus = "renamed"
	FileCopied    FileStatus = "copied"
	FileChanged   FileStatus = "changed"
	FileUnchanged FileStatus = "unchanged"
)

// CommitFile is a file that a commit changed. PreviousPath is set for a
// renamed or copied file. SHA names the file's blob after the commit.
//
// Patch is the unified diff of the file, without the header. GitHub leaves
// it out for binary files and for diffs too large to show, and
// PatchTruncated tells the second case: lines changed, but there is no
// patch.
type CommitFile struct {
	Path           string
	PreviousPath   string
	Status         FileStatus
	SHA            string
	Additions      int
	Deletions      int
	Patch          string
	PatchTruncated bool
}

// CompareStatus is how two commits relate.
type CompareStatus string

// Compare statuses.
const (
	CompareIdentical CompareStatus = "identical"
	CompareAhead     CompareStatus = "ahead"
	CompareBehind    CompareStatus = "behind"
	CompareDiverged  CompareStatus = "diverged"
)

// Compare is how far head is from base: AheadBy commits that base lacks,
// and BehindBy commits that head lacks.
type Compare struct {
	Status   CompareStatus
	AheadBy  int
	BehindBy int
}
