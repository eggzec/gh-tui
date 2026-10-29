package search

import (
	"context"
	"errors"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

type fakePreviewAPI struct {
	profiles, commits, patches int
	err                        error
}

func (f *fakePreviewAPI) GetUserProfile(_ context.Context, login string) (core.UserProfile, error) {
	f.profiles++
	return core.UserProfile{Login: login}, f.err
}

func (f *fakePreviewAPI) GetCommit(context.Context, core.RepoRef, string) (core.CommitDetail, error) {
	f.commits++
	return core.CommitDetail{Files: []core.CommitFile{{Path: "a", Patch: "@@"}}}, f.err
}

func (f *fakePreviewAPI) CommitPatch(context.Context, core.RepoRef, string) (patch string, tooLarge bool, err error) {
	f.patches++
	return "diff", false, f.err
}

func TestPreviewsKeepWhatTheyRead(t *testing.T) {
	t.Parallel()
	api := &fakePreviewAPI{}
	p := NewPreviews(api)
	ref := core.RepoRef{Owner: "o", Name: "r"}
	for range 2 {
		if got, err := p.Profile(t.Context(), "Octo"); err != nil || got.Login != "Octo" {
			t.Fatalf("Profile = %+v, %v", got, err)
		}
		got, err := p.Commit(t.Context(), ref, "abc")
		if err != nil || got.Patch != "diff" || len(got.Detail.Files) != 1 || got.Detail.Files[0].Patch != "" {
			t.Fatalf("Commit = %+v, %v, want the patch whole and the files without theirs", got, err)
		}
	}
	if api.profiles != 1 || api.commits != 1 || api.patches != 1 {
		t.Errorf("reads = %d profiles, %d commits, %d patches, want one each", api.profiles, api.commits, api.patches)
	}
}

func TestPreviewsBoundTheCommitCache(t *testing.T) {
	t.Parallel()
	api := &fakePreviewAPI{}
	p := NewPreviews(api)
	ref := core.RepoRef{Owner: "o", Name: "r"}
	for i := range commitEntries + 1 {
		if _, err := p.Commit(t.Context(), ref, string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Commit(t.Context(), ref, "a"); err != nil {
		t.Fatal(err)
	}
	if want := commitEntries + 2; api.commits != want {
		t.Errorf("commit reads = %d, want %d: the oldest entry should have been dropped", api.commits, want)
	}
}

func TestPreviewsFail(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	p := NewPreviews(&fakePreviewAPI{err: boom})
	if _, err := p.Profile(t.Context(), "x"); !errors.Is(err, boom) {
		t.Errorf("Profile error = %v, want boom", err)
	}
	if _, err := p.Commit(t.Context(), core.RepoRef{}, "abc"); !errors.Is(err, boom) {
		t.Errorf("Commit error = %v, want boom", err)
	}
}
