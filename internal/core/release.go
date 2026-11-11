package core

import "time"

// Release is a release of a repository: a tag with notes and the files
// uploaded with it.
type Release struct {
	// ID is what the API names the release by.
	ID   int64
	Tag  string
	Name string
	// Target is the branch or commit the tag was made from.
	Target string
	// Latest reports that this is the repository's latest release, which a
	// list read marks and a single read marks when it knows.
	Latest bool
	// Author is who published it, often a bot.
	Author User
	// Body is the release notes, in markdown.
	Body string
	// URL is the page of the release.
	URL        string
	Draft      bool
	Prerelease bool
	CreatedAt  time.Time
	// PublishedAt is zero for a draft.
	PublishedAt time.Time
	Assets      []ReleaseAsset
}

// ReleaseAsset is a file uploaded with a release.
type ReleaseAsset struct {
	// ID is what the API names the file by.
	ID   int64
	Name string
	// Size is in bytes.
	Size      int64
	Downloads int
	// URL downloads it.
	URL string
}
