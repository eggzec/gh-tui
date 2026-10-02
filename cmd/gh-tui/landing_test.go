package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
)

type landingRepos struct {
	err   error
	fresh bool
}

func (r *landingRepos) Get(context.Context, core.RepoRef) (core.Repo, error) {
	return core.Repo{}, r.err
}

func (r *landingRepos) FreshGet(core.RepoRef) bool { return r.fresh }

type landingFiles struct {
	asked  []filesvc.TreeQuery
	err    error
	cached bool
}

func (f *landingFiles) All(_ context.Context, q filesvc.TreeQuery) (core.Tree, error) {
	f.asked = append(f.asked, q)
	return core.Tree{}, f.err
}

func (f *landingFiles) CachedAll(filesvc.TreeQuery) (core.Tree, bool) { return core.Tree{}, f.cached }

func TestLandingRead(t *testing.T) {
	repo := core.RepoRef{Owner: "cli", Name: "cli"}
	failed := errors.New("boom")
	limited := fmt.Errorf("get repo: %w", core.ErrRateLimited)
	for _, tt := range []struct {
		name      string
		repoErr   error
		filesErr  error
		wantFiles bool
		want      error
	}{
		{"both", nil, nil, true, nil},
		// A repository gone or refused has no listing to read.
		{"repo failed", failed, nil, false, failed},
		{"listing failed", nil, failed, true, failed},
		{"rate limited", limited, nil, false, core.ErrRateLimited},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := &landingFiles{err: tt.filesErr}
			l := landing{repos: &landingRepos{err: tt.repoErr}, files: files}
			err := l.Read(t.Context(), repo)
			if tt.want == nil && err != nil || tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("Read = %v, want %v", err, tt.want)
			}
			if got := len(files.asked) == 1; got != tt.wantFiles {
				t.Fatalf("listing read %v, want %v", got, tt.wantFiles)
			}
			if tt.wantFiles && files.asked[0] != (filesvc.TreeQuery{Repo: repo}) {
				t.Errorf("listing read %+v, want the default branch of %s", files.asked[0], repo)
			}
		})
	}
}

// Only a fresh repository with its listing in memory counts as read.
func TestLandingCached(t *testing.T) {
	repo := core.RepoRef{Owner: "cli", Name: "cli"}
	for _, tt := range []struct {
		fresh, cached, want bool
	}{
		{true, true, true},
		{false, true, false},
		{true, false, false},
	} {
		l := landing{repos: &landingRepos{fresh: tt.fresh}, files: &landingFiles{cached: tt.cached}}
		if got := l.Cached(repo); got != tt.want {
			t.Errorf("Cached with a fresh repository %v and the listing cached %v = %v, want %v", tt.fresh, tt.cached, got, tt.want)
		}
	}
}
