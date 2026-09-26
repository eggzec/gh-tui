// Package core holds the domain types shared by services and the tui.
// It has no I/O and no dependencies outside the standard library.
package core

import (
	"fmt"
	"strings"
	"time"
)

// RepoRef identifies a repository by owner and name.
type RepoRef struct {
	Owner string
	Name  string
}

// ParseRepoRef parses "owner/name". It keeps the case the caller used and
// rejects characters GitHub doesn't allow in an owner or a name.
func ParseRepoRef(s string) (RepoRef, error) {
	owner, name, ok := strings.Cut(s, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return RepoRef{}, fmt.Errorf("invalid repo %q: want owner/name", s)
	}
	if err := checkOwner(owner); err != nil {
		return RepoRef{}, fmt.Errorf("invalid repo %q: %w", s, err)
	}
	if err := checkName(name); err != nil {
		return RepoRef{}, fmt.Errorf("invalid repo %q: %w", s, err)
	}
	return RepoRef{Owner: owner, Name: name}, nil
}

// maxNameLen is the longest repository name GitHub accepts.
const maxNameLen = 100

// checkOwner allows '_' because Enterprise managed users have logins such
// as "octocat_acme".
func checkOwner(s string) error {
	switch {
	case strings.HasPrefix(s, "-"):
		return fmt.Errorf("owner %q may not start with '-'", s)
	case !onlyChars(s, "-_"):
		return fmt.Errorf("owner %q may hold only letters, digits, '-' and '_'", s)
	}
	return nil
}

// checkName rejects "." and ".." because GitHub does; they would also turn
// into path traversal in a URL.
func checkName(s string) error {
	switch {
	case len(s) > maxNameLen:
		return fmt.Errorf("name is longer than %d characters", maxNameLen)
	case s == "." || s == "..":
		return fmt.Errorf("name may not be %q", s)
	case !onlyChars(s, ".-_"):
		return fmt.Errorf("name %q may hold only letters, digits, '.', '-' and '_'", s)
	}
	return nil
}

// onlyChars reports whether s holds only ASCII letters, digits and extra.
func onlyChars(s, extra string) bool {
	for _, c := range []byte(s) {
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case strings.IndexByte(extra, c) >= 0:
		default:
			return false
		}
	}
	return true
}

func (r RepoRef) String() string {
	return r.Owner + "/" + r.Name
}

// Repo is a GitHub repository. ID is the GraphQL node ID. Starred reports
// whether the viewer has starred it. LanguageColor is the hex color GitHub
// gives Language, such as "#00ADD8", or empty when the read didn't say.
// Caps are known only when the repository was read on its own, not in a
// list.
type Repo struct {
	ID            string
	Ref           RepoRef
	Description   string
	DefaultBranch string
	Language      string
	LanguageColor string
	Stars         int
	Starred       bool
	Private       bool
	Fork          bool
	Archived      bool
	Template      bool
	Mirror        bool
	UpdatedAt     time.Time
	URL           string
	Caps          RepoCaps
}
