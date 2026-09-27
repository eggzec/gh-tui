package github

import "github.com/eggzec/gh-tui/internal/buildinfo"

// userAgent names gh-tui and its version to GitHub, which asks every
// client to name itself.
var userAgent = agent(buildinfo.Version())

// agent returns the user agent of version, the version of the module the
// binary was built from. A build with no version, such as go run's, is
// named without one.
func agent(version string) string {
	if version == "" || version == "(devel)" {
		return "gh-tui"
	}
	return "gh-tui/" + version
}
