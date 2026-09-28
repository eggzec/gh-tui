package ui

import (
	"cmp"

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
