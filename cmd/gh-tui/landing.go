package main

import (
	"context"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
)

// landing reads what the repository screen reads first when the dashboard
// or the page of an owner opens a repository: the repository, which the header shows, and the
// listing of its default branch, which the files tree shows.
type landing struct {
	repos interface {
		Get(ctx context.Context, repo core.RepoRef) (core.Repo, error)
		FreshGet(repo core.RepoRef) bool
	}
	files interface {
		All(ctx context.Context, q filesvc.TreeQuery) (core.Tree, error)
		CachedAll(q filesvc.TreeQuery) (core.Tree, bool)
	}
}

// Read reads the repository and the listing of its default branch. A
// repository that can't be read, such as one gone or refused, has no
// listing worth reading.
func (l landing) Read(ctx context.Context, repo core.RepoRef) error {
	if _, err := l.repos.Get(ctx, repo); err != nil {
		return err
	}
	_, err := l.files.All(ctx, filesvc.TreeQuery{Repo: repo})
	return err
}

// Cached reports whether the repository is fresh in memory and the
// listing of its default branch is in memory. A stale listing costs a
// conditional request, which the repository screen sends anyway.
func (l landing) Cached(repo core.RepoRef) bool {
	_, ok := l.files.CachedAll(filesvc.TreeQuery{Repo: repo})
	return ok && l.repos.FreshGet(repo)
}
