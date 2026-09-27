package ui

import (
	"cmp"

	"github.com/eggzec/gh-tui/internal/core"
)

// WebURL returns the address of path, escaped already, on the web pages of
// host: the user's GitHub host with its port, if it has one, such as
// github.com or an Enterprise Server's host. An empty host is github.com.
func WebURL(host, path string) string {
	return "https://" + cmp.Or(host, core.DefaultHost) + "/" + path
}
