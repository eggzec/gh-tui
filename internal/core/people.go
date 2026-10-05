package core

// MemberRole is a member's role in an organization.
type MemberRole int

const (
	// MemberRoleUnknown is a role the viewer may not see, as for someone
	// outside the organization, the zero value.
	MemberRoleUnknown MemberRole = iota
	// MemberRoleMember is a member who doesn't own the organization.
	MemberRoleMember
	// MemberRoleAdmin is an owner of the organization.
	MemberRoleAdmin
)

// Person is a row of a list of accounts: a user's followers, the accounts
// they follow, their organizations, an organization's members, or
// sponsors.
type Person struct {
	Kind  OwnerKind
	Login string
	Name  string
	// Bio is a user's bio, or an organization's description.
	Bio string
	// Role is a member's role, in a list of an organization's members.
	Role MemberRole
}

// Team is a team of an organization.
type Team struct {
	Name        string
	Slug        string
	Description string
	// Secret reports a team only its members and the organization's
	// owners see.
	Secret bool
	// Members counts the team's members.
	Members int
	URL     string
}

// Readme is an owner's profile README.
type Readme struct {
	// Markdown is the README's content.
	Markdown string
	// Source is the repository the README is in, the zero value if the
	// owner has none. Links relative to the README resolve against it.
	Source RepoRef
	// MembersOnly reports an organization's README that only its members
	// see.
	MembersOnly bool
}
