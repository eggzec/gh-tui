package main

import (
	"fmt"

	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/eggzec/gh-tui/internal/core"
)

// startRepo picks the repository the app opens with: the one named on the
// command line, else the one in the current directory, else the first
// pinned one. It returns the zero RepoRef when there is none, and the app
// asks the user to pick one.
func startRepo(arg string, current func() (core.RepoRef, bool), pinned []core.RepoRef) (core.RepoRef, error) {
	if arg != "" {
		ref, err := core.ParseRepoRef(arg)
		if err != nil {
			return core.RepoRef{}, fmt.Errorf("repository argument: %w", err)
		}
		return ref, nil
	}
	if ref, ok := current(); ok {
		return ref, nil
	}
	if len(pinned) > 0 {
		return pinned[0], nil
	}
	return core.RepoRef{}, nil
}

// currentRepo reads the repository of the current directory from its git
// remotes, the way gh does. Outside a repository it reports false.
func currentRepo() (core.RepoRef, bool) {
	r, err := repository.Current()
	if err != nil {
		return core.RepoRef{}, false
	}
	return core.RepoRef{Owner: r.Owner, Name: r.Name}, true
}
