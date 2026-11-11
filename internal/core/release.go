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
	// AuthorAvatar is the address of the author's picture, empty if the
	// release names none.
	AuthorAvatar string
	// Immutable reports that the release can no longer be changed. Servers
	// that don't say read as false.
	Immutable bool
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

// ReleaseExtra is what a release's header shows beyond the release itself,
// read with requests of their own.
type ReleaseExtra struct {
	// TagSHA is the commit the tag names.
	TagSHA string
	// Verified reports that GitHub verified the signature of that commit.
	Verified bool
	// AheadBy is how many commits the default branch has since the tag, or
	// -1 if that is unknown, such as for a deleted tag.
	AheadBy int
	// Default is the name of the default branch, empty if unknown.
	Default string
	// Mentions are the logins the notes mention, in order of appearance.
	Mentions []string
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
