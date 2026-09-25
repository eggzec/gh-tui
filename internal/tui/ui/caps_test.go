package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// fakeRepos serves one repository, and fails the reads of others.
type fakeRepos struct {
	repo   core.Repo
	cached bool
	gets   int
}

func (f *fakeRepos) CachedGet(ref core.RepoRef) (core.Repo, bool) {
	if !f.cached || ref != f.repo.Ref {
		return core.Repo{}, false
	}
	return f.repo, true
}

func (f *fakeRepos) Get(_ context.Context, ref core.RepoRef) (core.Repo, error) {
	f.gets++
	if ref != f.repo.Ref {
		return core.Repo{}, errors.New("not found")
	}
	f.cached = true
	return f.repo, nil
}

func TestCaps(t *testing.T) {
	ref := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	caps := core.RepoCaps{Known: true, Permission: core.PermissionRead}
	src := &fakeRepos{repo: core.Repo{Ref: ref, Caps: caps}}

	if c := CachedCaps(nil, ref); c.Known {
		t.Errorf("CachedCaps without a source = %+v, want unknown", c)
	}
	if LoadCaps(t.Context(), nil, ref) != nil {
		t.Error("LoadCaps without a source returned a command")
	}
	if c := CachedCaps(src, ref); c.Known || src.gets != 0 {
		t.Errorf("CachedCaps before a read = %+v after %d reads, want unknown without one", c, src.gets)
	}
	if msg := LoadCaps(t.Context(), src, ref)(); msg != (CapsMsg{Repo: ref, Caps: caps}) {
		t.Errorf("LoadCaps reports %+v, want the caps", msg)
	}
	if c := CachedCaps(src, ref); c != caps {
		t.Errorf("CachedCaps after the read = %+v, want %+v", c, caps)
	}
	if msg := LoadCaps(t.Context(), src, core.RepoRef{Owner: "a", Name: "b"})(); msg != nil {
		t.Errorf("a failed LoadCaps reports %+v, want nothing", msg)
	}
}
