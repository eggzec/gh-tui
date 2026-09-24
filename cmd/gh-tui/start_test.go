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
}

func (f *fakeLister) List(context.Context, reposvc.ListQuery) (core.Page[core.Repo], error) {
	return core.Page[core.Repo]{Items: f.repos}, f.err
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
