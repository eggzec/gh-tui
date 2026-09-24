package main

import (
	"fmt"

	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/eggzec/gh-tui/internal/core"
)

// startRepos picks what the app opens with. open is the repository named
// on the command line, which the app opens on; without one it opens on the
// dashboard. here is the repository of the current directory, which the
// dashboard shows first. Either is the zero RepoRef when there is none.
func startRepos(arg string, current func() (core.RepoRef, bool)) (open, here core.RepoRef, err error) {
	if arg != "" {
		open, err = core.ParseRepoRef(arg)
		if err != nil {
			return core.RepoRef{}, core.RepoRef{}, fmt.Errorf("repository argument: %w", err)
		}
	}
	here, _ = current()
	return open, here, nil
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
