package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
)

type fakeLister struct {
	repos []core.Repo
	err   error
	// cached is what CachedGet finds, by repoID.
	cached map[string]core.Repo
	// again records the Again of each list.
	again []bool
}

func (f *fakeLister) List(_ context.Context, q reposvc.ListQuery) (core.Page[core.Repo], error) {
	f.again = append(f.again, q.Again)
	return core.Page[core.Repo]{Items: f.repos}, f.err
}

func (f *fakeLister) CachedGet(ref core.RepoRef) (core.Repo, bool) {
	r, ok := f.cached[repoID(ref)]
	return r, ok
}

// The first list may show the repositories an earlier session kept, and
// every later one reads past them.
func TestSearchStartReadsPastKeptList(t *testing.T) {
	repos := &fakeLister{}
	start := searchStart(repos, nil)
	for range 3 {
		if _, err := start(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if want := []bool{false, true, true}; !slices.Equal(repos.again, want) {
		t.Errorf("listed with Again %v, want %v", repos.again, want)
	}
}

var (
	ghTUI     = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	bubbletea = core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
)

func TestSearchStart(t *testing.T) {
	dotfiles := core.RepoRef{Owner: "eggzec", Name: "dotfiles"}
	repos := &fakeLister{repos: []core.Repo{{Ref: ghTUI, Description: "own"}, {Ref: dotfiles}}}
	got, err := searchStart(repos, []core.RepoRef{ghTUI, bubbletea})(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	refs := make([]core.RepoRef, len(got))
	for i, r := range got {
		refs[i] = r.Ref
	}
	if want := []core.RepoRef{ghTUI, bubbletea, dotfiles}; !slices.Equal(refs, want) {
		t.Errorf("start = %v, want the pinned ones first, then the viewer's own without them", refs)
	}

	failing := &fakeLister{err: core.ErrRateLimited}
	if _, err := searchStart(failing, nil)(t.Context()); err == nil || !strings.Contains(err.Error(), "slow down") {
		t.Errorf("error = %v, want the rate limit reworded", err)
	}
	if _, err := searchStart(&fakeLister{err: errors.New("boom")}, nil)(t.Context()); err == nil || !strings.Contains(err.Error(), "list your repositories") {
		t.Errorf("error = %v, want it wrapped", err)
	}
}

// The config may spell a repository otherwise than GitHub does. It shows
// once, as GitHub spells it.
func TestSearchStartIgnoresCase(t *testing.T) {
	spelled := core.RepoRef{Owner: "Eggzec", Name: "Gh-Tui"}
	cli := core.RepoRef{Owner: "cli", Name: "cli"}
	repos := &fakeLister{
		repos:  []core.Repo{{Ref: ghTUI}},
		cached: map[string]core.Repo{"cli/cli": {Ref: cli}},
	}
	pinned := []core.RepoRef{spelled, {Owner: "CLI", Name: "Cli"}, {Owner: "cli", Name: "CLI"}, bubbletea}
	got, err := searchStart(repos, pinned)(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	refs := make([]core.RepoRef, len(got))
	for i, r := range got {
		refs[i] = r.Ref
	}
	if want := []core.RepoRef{ghTUI, cli, bubbletea}; !slices.Equal(refs, want) {
		t.Errorf("start = %v, want %v", refs, want)
	}
}

func TestFriendly(t *testing.T) {
	reset := time.Date(2026, 9, 24, 15, 4, 0, 0, time.Local)
	tests := []struct {
		err  error
		want string
	}{
		{&core.RateLimitError{Reset: reset}, "try again at 3:04PM"},
		{core.ErrRateLimited, "try again in a minute"},
		{errors.New("boom"), "boom"},
	}
	for _, tt := range tests {
		if err := friendly(tt.err); !strings.Contains(err.Error(), tt.want) {
			t.Errorf("friendly(%v) = %v, want it to say %q", tt.err, err, tt.want)
		}
	}
}
