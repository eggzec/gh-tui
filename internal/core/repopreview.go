package core

// RepoPreview is what the search preview of a repository reads beyond the
// hit: its README, how many issues and pull requests are open, and its
// latest release.
type RepoPreview struct {
	// Readme is the README as Markdown, empty if there is none.
	Readme     string
	OpenIssues int
	OpenPulls  int
	// Release is the latest release; its Tag is empty if there is none.
	Release Release
}
