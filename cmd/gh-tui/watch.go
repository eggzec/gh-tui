package main

import (
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/watch"
)

// repoPoll polls one kind of data of a repository, such as its pull
// requests, under the key that the matching section listens for.
type repoPoll struct {
	key  func(repo core.RepoRef) string
	poll func(repo core.RepoRef) watch.PollFunc
}

// repoWatch keeps the polls of the selected repository subscribed, and only
// those. It is not safe for concurrent use: build sets the first repository
// before the program runs, and the app sets the others from Update.
type repoWatch struct {
	subscribe func(key string, fn watch.PollFunc) (unsubscribe func())
	polls     []repoPoll

	repo  core.RepoRef
	unsub []func()
}

// set switches the polls over to repo. The zero RepoRef polls nothing.
func (w *repoWatch) set(repo core.RepoRef) {
	if repo == w.repo {
		return
	}
	for _, unsub := range w.unsub {
		unsub()
	}
	w.unsub, w.repo = nil, repo
	if repo == (core.RepoRef{}) {
		return
	}
	for _, p := range w.polls {
		w.unsub = append(w.unsub, w.subscribe(p.key(repo), p.poll(repo)))
	}
}
