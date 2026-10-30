package ui

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"net/url"

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

// ProfileURL returns the page of the user or bot login on host, or "" for
// no login.
func ProfileURL(host, login string) string {
	if login == "" {
		return ""
	}
	return WebURL(host, url.PathEscape(login))
}

// PageProfileURL returns the page of login on the host of page, the address
// of a pull request or an issue, for a view that has no host of its own. It
// returns "" when page has no host or login is empty.
func PageProfileURL(page, login string) string {
	u, err := url.Parse(page)
	if err != nil || u.Host == "" {
		return ""
	}
	return ProfileURL(u.Host, login)
}

// DiffAnchor returns the anchor of the diff of the file at path on a page
// that shows diffs, such as a commit's or a pull request's files: GitHub
// names it by the SHA-256 of the path.
func DiffAnchor(path string) string {
	sum := sha256.Sum256([]byte(path))
	return "diff-" + hex.EncodeToString(sum[:])
}
