package core

import "slices"

// Permission is the role of the viewer in a repository. The empty
// Permission is unknown: GitHub doesn't say for a GitHub App.
type Permission string

// Permissions, from the weakest.
const (
	PermissionRead     Permission = "read"
	PermissionTriage   Permission = "triage"
	PermissionWrite    Permission = "write"
	PermissionMaintain Permission = "maintain"
	PermissionAdmin    Permission = "admin"
)

func (p Permission) rank() int {
	switch p {
	case PermissionRead:
		return 1
	case PermissionTriage:
		return 2
	case PermissionWrite:
		return 3
	case PermissionMaintain:
		return 4
	case PermissionAdmin:
		return 5
	}
	return 0
}

// AtLeast reports whether p grants all that q does. An unknown p grants
// nothing.
func (p Permission) AtLeast(q Permission) bool {
	return p.rank() > 0 && p.rank() >= q.rank()
}

// RepoCaps is what the viewer may do in a repository, as GitHub said when
// the repository was read. The zero value is unknown, with Known unset:
// nothing is refused on its account, and GitHub has the last word.
type RepoCaps struct {
	Known bool
	// Permission is the viewer's role, empty when GitHub doesn't say.
	Permission Permission
	// Archived and Locked repositories are read-only for everyone.
	Archived bool
	Locked   bool
	// Private repositories take a token with the repo scope to change.
	Private bool
	// Issues, PullRequests, Discussions, Projects and Wiki say which
	// features are turned on.
	Issues, PullRequests, Discussions, Projects, Wiki bool
	// MergeCommit, Squash and Rebase say how pull requests may be merged,
	// and AutoMerge whether they may merge once their checks pass.
	MergeCommit, Squash, Rebase bool
	AutoMerge                   bool
	// DefaultMerge is the method the viewer merged with last, or the
	// repository's default.
	DefaultMerge MergeMethod
}

// ReadOnly reports whether the viewer may change nothing in the
// repository: it is archived or locked, or the viewer may only read it.
// Unknown caps are not read-only.
func (c RepoCaps) ReadOnly() bool {
	return c.Known && (c.Archived || c.Locked || !c.grants(PermissionTriage))
}

// CanTriage reports whether the viewer may triage issues and pull
// requests, such as closing and labeling them. Unknown caps may.
func (c RepoCaps) CanTriage() bool {
	return !c.Known || (!c.Archived && !c.Locked && c.grants(PermissionTriage))
}

// CanWrite reports whether the viewer may push, merge and run workflows.
// Unknown caps may.
func (c RepoCaps) CanWrite() bool {
	return !c.Known || (!c.Archived && !c.Locked && c.grants(PermissionWrite))
}

// grants reports whether the viewer's permission grants p, taking an
// unknown permission for one that does.
func (c RepoCaps) grants(p Permission) bool {
	return c.Permission == "" || c.Permission.AtLeast(p)
}

// MergeMethods returns the methods pull requests may be merged with, in
// the order GitHub offers them. Unknown caps allow them all.
func (c RepoCaps) MergeMethods() []MergeMethod {
	if !c.Known {
		return []MergeMethod{MergeCommit, MergeSquash, MergeRebase}
	}
	var out []MergeMethod
	for _, m := range []struct {
		method MergeMethod
		ok     bool
	}{{MergeCommit, c.MergeCommit}, {MergeSquash, c.Squash}, {MergeRebase, c.Rebase}} {
		if m.ok {
			out = append(out, m.method)
		}
	}
	return out
}

// MergeMethod returns the method to merge with when the user prefers
// preferred: that one if the repository allows it, or else the viewer's
// default, or else squash, or else the first allowed. It reports false
// when the repository allows none.
func (c RepoCaps) MergeMethod(preferred MergeMethod) (MergeMethod, bool) {
	allowed := c.MergeMethods()
	for _, m := range []MergeMethod{preferred, c.DefaultMerge} {
		if slices.Contains(allowed, m) {
			return m, true
		}
	}
	switch {
	case slices.Contains(allowed, MergeSquash):
		return MergeSquash, true
	case len(allowed) == 0:
		return "", false
	}
	return allowed[0], true
}

// ItemCaps is what the viewer may do to an issue or pull request, as
// GitHub said when it was read. Known is unset when the read didn't say,
// as the REST reads of issues don't; the caps of the repository decide
// then.
type ItemCaps struct {
	Known bool
	// Update is editing it, which covers converting a pull request to a
	// draft, Close and Reopen changing its state, and Label its labels.
	Update, Close, Reopen, Label bool
	// LabelKnown is set when GitHub said Label, which a GitHub Enterprise
	// Server older than 3.15 doesn't; triage access to the repository
	// decides it then.
	LabelKnown bool
	// Authored is set when the viewer opened it.
	Authored bool
}
