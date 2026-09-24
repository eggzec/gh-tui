package core

import (
	"bytes"
	"fmt"
)

// EntryType is the kind of object a tree entry points at.
type EntryType string

// Entry types, as git names them.
const (
	EntryBlob EntryType = "blob"
	EntryTree EntryType = "tree"
	// EntryCommit is a submodule: a commit in another repository.
	EntryCommit EntryType = "commit"
)

// ModeSymlink is the git file mode of a symbolic link.
const ModeSymlink = "120000"

// TreeEntry is one entry of a git tree. Path is relative to the tree that
// was listed, so in a listing of one level it equals Name, and in a
// recursive listing it holds the directories on the way, separated by "/".
type TreeEntry struct {
	Path string
	Name string
	Type EntryType
	// Mode is the git file mode, such as 100644 or 040000.
	Mode string
	SHA  string
	// Size is the size of a blob in bytes, and 0 for other entries.
	Size int64
}

// Dir reports whether the entry is a directory.
func (e TreeEntry) Dir() bool {
	return e.Type == EntryTree
}

// Submodule reports whether the entry is a submodule, which has no content
// in this repository.
func (e TreeEntry) Submodule() bool {
	return e.Type == EntryCommit
}

// Symlink reports whether the entry is a symbolic link. Its blob holds the
// link's target.
func (e TreeEntry) Symlink() bool {
	return e.Mode == ModeSymlink
}

// Tree is a listing of a git tree. SHA is the tree's own SHA, even when it
// was asked for by a ref. Truncated reports that GitHub left entries out
// because the tree is too large; list its subtrees one at a time instead.
type Tree struct {
	SHA       string
	Entries   []TreeEntry
	Truncated bool
}

// Blob is the content of a file.
type Blob struct {
	SHA     string
	Size    int64
	Content []byte
	// Binary reports that the content doesn't look like text, so it
	// shouldn't be shown as is.
	Binary bool
}

// binarySniffLen is how much of a blob LooksBinary reads, as git does.
const binarySniffLen = 8000

// LooksBinary reports whether b looks like binary data rather than text: it
// has a NUL byte near the start. Git uses the same test.
func LooksBinary(b []byte) bool {
	return bytes.IndexByte(b[:min(len(b), binarySniffLen)], 0) >= 0
}

// TooLargeError reports that a blob is larger than the limit for reading it.
// Size is 0 when the size isn't known. It matches ErrTooLarge.
type TooLargeError struct {
	Size  int64
	Limit int64
}

func (e *TooLargeError) Error() string {
	if e.Size <= 0 {
		return fmt.Sprintf("larger than %d bytes", e.Limit)
	}
	return fmt.Sprintf("%d bytes is larger than %d", e.Size, e.Limit)
}

// Is reports whether target is ErrTooLarge.
func (e *TooLargeError) Is(target error) bool {
	return target == ErrTooLarge
}
