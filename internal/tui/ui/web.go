package ui

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"

	"github.com/eggzec/gh-tui/internal/core"
)

// WebURL returns the address of path, escaped already, on the web pages of
// host: the user's GitHub host with its port, if it has one, such as
// github.com or an Enterprise Server's host, with its scheme
// (core.WebScheme). An empty host is github.com.
func WebURL(host, path string) string {
	host = cmp.Or(host, core.DefaultHost)
	return core.WebScheme(host) + "://" + host + "/" + path
}

// DiffAnchor returns the anchor of the diff of the file at path on a page
// that shows diffs, such as a commit's or a pull request's files: GitHub
// names it by the SHA-256 of the path.
func DiffAnchor(path string) string {
	sum := sha256.Sum256([]byte(path))
	return "diff-" + hex.EncodeToString(sum[:])
}
