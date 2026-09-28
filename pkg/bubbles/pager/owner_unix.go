//go:build unix

package pager

import (
	"os"
	"syscall"
)

// ownedByUser reports whether the user the program runs as owns fi.
func ownedByUser(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}
