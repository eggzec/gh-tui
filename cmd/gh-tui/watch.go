package main

import (
	"context"

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

// unless returns poll, which does nothing while skip reports true for the
// repository, as there is nothing to poll: GitHub still answers for the
// issues of a repository that turned them off.
func unless(skip func(repo core.RepoRef) bool, poll func(repo core.RepoRef) watch.PollFunc) func(repo core.RepoRef) watch.PollFunc {
	return func(repo core.RepoRef) watch.PollFunc {
		fn := poll(repo)
		return func(ctx context.Context) (watch.Result, error) {
			if skip(repo) {
				return watch.Result{}, nil
			}
			return fn(ctx)
		}
	}
}

// issuesOff reports whether a repository turned its issues off, as far as
// what cached knows of it tells.
func issuesOff(cached func(ref core.RepoRef) (core.Repo, bool)) func(repo core.RepoRef) bool {
	return func(repo core.RepoRef) bool {
		r, _ := cached(repo)
		return r.Caps.Known && !r.Caps.Issues
	}
}

// refreshOn polls key at once each time what the token may do changes,
// until ctx is done.
func refreshOn(ctx context.Context, changes <-chan core.Access, refresh func(key string), key string) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-changes:
				refresh(key)
			}
		}
	}()
}
