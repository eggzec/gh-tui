package core

import (
	"cmp"
	"slices"
	"strings"
)

// TokenKind is the kind of token the app acts with, which says what
// GitHub tells of what it may do.
type TokenKind uint8

// The kinds of token.
const (
	// TokenUnknown is a token of no kind the app knows.
	TokenUnknown TokenKind = iota
	// TokenClassic is an OAuth token, such as gh's (gho_), or a classic
	// personal access token (ghp_): GitHub lists its scopes.
	TokenClassic
	// TokenFineGrained is a fine-grained personal access token, whose
	// permissions are per repository and never listed.
	TokenFineGrained
	// TokenApp is a GitHub App's token, for a user (ghu_) or an
	// installation (ghs_).
	TokenApp
)

func (k TokenKind) String() string {
	switch k {
	case TokenClassic:
		return "classic"
	case TokenFineGrained:
		return "fine-grained"
	case TokenApp:
		return "app"
	default:
		return "unknown"
	}
}

// Access is what the token may do, as far as GitHub said. The zero value
// is unknown and allows everything: GitHub has the last word.
type Access struct {
	Kind TokenKind
	// Known says that Scopes came from GitHub. Without it the scopes are
	// unknown, and nothing is refused on their account.
	Known bool
	// Scopes are the scopes of a classic token, sorted.
	Scopes []string
	// SSO are the ids of the organizations that left their results out
	// until the token is authorized for their single sign-on.
	SSO []string
}

// Equal reports whether a and b say the same.
func (a Access) Equal(b Access) bool {
	return a.Kind == b.Kind && a.Known == b.Known &&
		slices.Equal(a.Scopes, b.Scopes) && slices.Equal(a.SSO, b.SSO)
}

// Need is what an operation needs of a token.
type Need struct {
	// Scopes are the scopes of which a classic token needs any one, the
	// one to ask for first. Empty, it needs none.
	Scopes []string
	// Classic says that the operation takes no other kind of token.
	Classic bool
}

// Allows reports whether the token may do what needs n, and whether that
// is known. What isn't known is allowed: an unknown token, the scopes of
// a classic one before GitHub listed them, and any scope of the other
// kinds, whose permissions can't be listed.
func (a Access) Allows(n Need) (ok, known bool) {
	switch {
	case a.Kind == TokenUnknown:
		return true, false
	case n.Classic && a.Kind != TokenClassic:
		return false, true
	case len(n.Scopes) == 0:
		return true, true
	case a.Kind != TokenClassic || !a.Known:
		return true, false
	}
	return slices.ContainsFunc(n.Scopes, func(want string) bool {
		return slices.ContainsFunc(a.Scopes, func(have string) bool { return Covers(have, want) })
	}), true
}

// Missing returns the scope to grant the token so that it may do what
// needs n, or "" when it may, when that isn't known, or when no scope
// would do, as for a token of another kind than a need for a classic one.
func (a Access) Missing(n Need) string {
	if ok, known := a.Allows(n); ok || !known || a.Kind != TokenClassic {
		return ""
	}
	return n.Scopes[0]
}

// ParseScopes splits a list of scopes, such as GitHub's X-OAuth-Scopes
// "gist, read:org, repo", and returns them sorted, without repeats. What
// isn't the name of a scope is left out, since a scope may end up in a
// command the user runs.
func ParseScopes(list string) []string {
	var out []string
	for s := range strings.SplitSeq(list, ",") {
		if s = strings.TrimSpace(s); scopeName(s) {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// scopeName reports whether s may be the name of a scope, such as
// read:org or admin:repo_hook.
func scopeName(s string) bool {
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != ':'
	})
}

// Covers reports whether a token with scope have may do what scope want
// allows. Scopes form a hierarchy: repo includes public_repo, repo:status,
// repo:invite, repo_deployment, security_events and admin:repo_hook; user
// includes read:user, user:email and user:follow; project includes
// read:project; and an admin: scope includes its write: scope, which
// includes its read: scope, as admin:org does write:org and read:org.
// Neither workflow nor notifications is part of repo.
func Covers(have, want string) bool {
	if have == want {
		return true
	}
	switch have {
	case "repo":
		switch want {
		case "public_repo", "security_events", "repo_deployment":
			return true
		}
		return strings.HasPrefix(want, "repo:") || Covers("admin:repo_hook", want)
	case "user":
		return strings.HasPrefix(want, "user:") || want == "read:user"
	case "project":
		return want == "read:project"
	case "admin:enterprise":
		switch want {
		case "manage_runners:enterprise", "manage_billing:enterprise":
			return true
		}
	}
	if name, ok := strings.CutPrefix(have, "admin:"); ok {
		return want == "write:"+name || want == "read:"+name
	}
	if name, ok := strings.CutPrefix(have, "write:"); ok {
		return want == "read:"+name
	}
	return false
}

// SortScopes returns scopes, of which any one would do, in the order to
// ask for them: the narrowest first. A scope that covers fewer of the
// others is narrower, and of those that cover none, one that has scopes
// of its own below it, such as repo, comes last, so that a list such as
// GitHub's "notifications, repo" asks for notifications.
func SortScopes(scopes []string) []string {
	out := slices.Clone(scopes)
	covered := func(s string) int {
		n := 0
		for _, t := range scopes {
			if t != s && Covers(s, t) {
				n++
			}
		}
		return n
	}
	slices.SortStableFunc(out, func(a, b string) int {
		return cmp.Or(cmp.Compare(covered(a), covered(b)), compareBool(parentScope(a), parentScope(b)))
	})
	return out
}

// parentScope reports whether scope includes other scopes.
func parentScope(scope string) bool {
	switch scope {
	case "repo", "user", "project":
		return true
	}
	return strings.HasPrefix(scope, "admin:") || strings.HasPrefix(scope, "write:")
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}
