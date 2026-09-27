package core

import "time"

// Status is what a user set as their status on GitHub. Both fields are
// empty if they set none.
type Status struct {
	// Emoji is GitHub's shortcode, such as ":coffee:".
	Emoji   string
	Message string
	// Busy reports that the user marked themselves as busy.
	Busy bool
}

// Profile is a user's public profile.
type Profile struct {
	Login    string
	Name     string
	Bio      string
	Company  string
	Location string
	// Website is the link the user added to their profile, and URL their
	// page on GitHub.
	Website   string
	URL       string
	AvatarURL string
	Followers int
	Following int
	// Repos counts the repositories the user owns.
	Repos     int
	Status    Status
	CreatedAt time.Time
}

// Org is an organization.
type Org struct {
	Login string
	Name  string
	URL   string
}

// Header is what the dashboard shows at its top: the viewer's profile, the
// repositories they pinned, in their order, and the organizations they
// belong to.
type Header struct {
	Profile Profile
	Pinned  []Repo
	Orgs    []Org
	// Stale reports that the header was kept by an earlier session and is
	// served before GitHub was asked again. Reading it again with the
	// query's Again set asks GitHub.
	Stale bool
	// Offline reports that GitHub couldn't be reached, so the header is the
	// one read last.
	Offline bool
	// Limited reports that GitHub rate limited the read, so the header is
	// the one read last.
	Limited bool
}

// WorkList is one list of work waiting on the viewer: how many items match
// in all, and the most recently updated of them. Items are hits of kind
// SearchPulls or SearchIssues.
type WorkList struct {
	Count int
	Items []SearchHit
}

// Work is the open work waiting on the viewer, across every repository.
type Work struct {
	// ReviewRequested holds the pull requests that ask for the viewer's
	// review, Authored those the viewer opened, and Assigned the issues
	// assigned to them. None holds archived repositories.
	ReviewRequested WorkList
	Authored        WorkList
	Assigned        WorkList
	// Stale, Offline and Limited work as in Header.
	Stale   bool
	Offline bool
	Limited bool
}

// ContributionDay is one day of a contribution calendar.
type ContributionDay struct {
	// Date is the day at midnight UTC.
	Date  time.Time
	Count int
	// Level ranks Count against the other days of the calendar, from 0 for
	// none to 4 for the busiest quarter, as GitHub shades its graph.
	Level int
}

// Contributions is the viewer's contribution calendar for the past year.
// Each week runs from Sunday to Saturday; the first and the last may be
// shorter.
type Contributions struct {
	Total int
	Weeks [][]ContributionDay
	// Stale, Offline and Limited work as in Header.
	Stale   bool
	Offline bool
	Limited bool
}
