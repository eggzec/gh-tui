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

// ParseRepoRef parses "owner/name".
func ParseRepoRef(s string) (RepoRef, error) {
	owner, name, ok := strings.Cut(s, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return RepoRef{}, fmt.Errorf("invalid repo %q: want owner/name", s)
	}
	return RepoRef{Owner: owner, Name: name}, nil
}

func (r RepoRef) String() string {
	return r.Owner + "/" + r.Name
}

// Repo is a GitHub repository. ID is the GraphQL node ID. Starred reports
// whether the viewer has starred it.
type Repo struct {
	ID            string
	Ref           RepoRef
	Description   string
	DefaultBranch string
	Language      string
	Stars         int
	Starred       bool
	Private       bool
	Fork          bool
	Archived      bool
	UpdatedAt     time.Time
	URL           string
}
