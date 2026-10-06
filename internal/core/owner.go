package core

// OwnerKind says whether an account is a user or an organization.
type OwnerKind int

// The kinds of owner.
const (
	OwnerUser OwnerKind = iota
	OwnerOrg
)

// Relation is how the viewer stands to an owner. The fields that don't
// apply to the owner's kind are false.
type Relation struct {
	// IsViewer reports that the owner is the signed-in user.
	IsViewer bool
	// Following reports that the viewer follows the owner, and CanFollow
	// that they may (users only).
	Following bool
	CanFollow bool
	// FollowsViewer reports that the user follows the viewer.
	FollowsViewer bool
	// Member reports that the viewer belongs to the organization, and
	// CanAdminister that they may administer it.
	Member        bool
	CanAdminister bool
}

// Owner is the top of a user's or an organization's page: the profile,
// the counts, the repositories pinned, in their order, and how the viewer
// stands to the owner.
type Owner struct {
	Kind OwnerKind
	// ID is the owner's GraphQL node ID.
	ID string
	// Profile holds the fields both kinds share. For an organization, Bio
	// holds its description, and Followers and Following are zero, since
	// GitHub's GraphQL API doesn't count them.
	Profile Profile
	// Pronouns and Stars, the repositories starred, are a user's only.
	Pronouns string
	Stars    int
	Twitter  string
	// Email, Verified, Members and Teams are an organization's only.
	// Verified reports that GitHub verified the organization's domains.
	Email    string
	Verified bool
	Members  int
	Teams    int
	Pinned   []Repo
	// HiddenPins reports that GitHub hid at least one pinned repository
	// from the token, as an organization's SAML enforcement does, so
	// Pinned holds only those it showed.
	HiddenPins bool
	Viewer     Relation
	// Stale, Offline and Limited work as in Header.
	Stale   bool
	Offline bool
	Limited bool
}

// RepoOrderField is what a list of repositories is sorted by.
type RepoOrderField int

const (
	// RepoOrderUpdated sorts by when a repository last changed, the zero
	// value and the default.
	RepoOrderUpdated RepoOrderField = iota
	// RepoOrderPushed sorts by the last push.
	RepoOrderPushed
	// RepoOrderCreated sorts by when a repository was created.
	RepoOrderCreated
	// RepoOrderName sorts by name.
	RepoOrderName
	// RepoOrderStars sorts by stars.
	RepoOrderStars
)

// RepoOrder is the order of a list of repositories. The zero value lists
// the most recently updated first.
type RepoOrder struct {
	Field     RepoOrderField
	Ascending bool
}
