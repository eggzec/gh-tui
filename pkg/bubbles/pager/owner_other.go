//go:build !unix

package pager

import "os"

// ownedByUser reports whether the user the program runs as owns fi. Where
// files have no owner the program can read, such as on Windows, whose
// temporary directory is the user's own, it reports true.
func ownedByUser(os.FileInfo) bool { return true }
