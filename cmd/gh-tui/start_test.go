package main

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
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
