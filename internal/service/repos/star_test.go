package repos

import (
	"errors"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

var other = core.Repo{ID: "R_3", Ref: core.RepoRef{Owner: "cli", Name: "go-gh"}, Stars: 300}

// seeded returns a service whose cache holds two list pages and the detail
// of ghTUI. The list page with ghTUI is backed by items, so tests can check
// that it isn't changed in place.
func seeded(t *testing.T, api *fakeAPI) (s *Service, items []core.Repo) {
	t.Helper()
	items = []core.Repo{ghTUI, dotfiles}
	api.listRepos = func(_ int, after string) (core.Page[core.Repo], error) {
		if after == "" {
			return core.Page[core.Repo]{Items: items, Next: "c1"}, nil
		}
		return page("", other), nil
	}
	api.getRepo = func(core.RepoRef) (core.Repo, error) { return ghTUI, nil }
	s = New(api)
	for _, q := range []ListQuery{{}, {After: "c1"}} {
		if _, err := s.List(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Get(t.Context(), ghTUI.Ref); err != nil {
		t.Fatal(err)
	}
	api.mu.Lock()
	api.calls = nil
	api.mu.Unlock()
	return s, items
}

// toggled returns r with its star flipped and stars stargazers.
func toggled(r core.Repo, stars int) core.Repo {
	r.Starred, r.Stars = !r.Starred, stars
	return r
}

// wantCached checks what the list pages and the detail of ghTUI show.
func wantCached(t *testing.T, s *Service, first []core.Repo, detail core.Repo) {
	t.Helper()
	if got, _ := s.CachedList(ListQuery{}); !slices.Equal(got.Items, first) || got.Next != "c1" {
		t.Errorf("first page = %+v, want %+v", got, first)
	}
	if got, _ := s.CachedList(ListQuery{After: "c1"}); !equalPage(got, page("", other)) {
		t.Errorf("second page = %+v, want it untouched", got)
	}
	if got, _ := s.CachedGet(ghTUI.Ref); got != detail {
		t.Errorf("detail = %+v, want %+v", got, detail)
	}
}

func TestStarShowsBeforeDo(t *testing.T) {
	api := &fakeAPI{t: t}
	s, items := seeded(t, api)

	op := s.Star(core.RepoRef{Owner: "EGGZEC", Name: "gh-tui"})

	want := toggled(ghTUI, 43)
	wantCached(t, s, []core.Repo{want, dotfiles}, want)
	api.wantCalls(t)
	if items[0] != ghTUI {
		t.Error("Star changed the cached page in place")
	}
	op.Rollback()
}

func TestStarRollsBackOnFailure(t *testing.T) {
	api := &fakeAPI{t: t}
	s, _ := seeded(t, api)
	api.star = func(core.RepoRef, bool) error { return core.ErrConflict }

	err := s.Star(ghTUI.Ref).Do(t.Context())

	if !errors.Is(err, core.ErrConflict) || err.Error() != "star eggzec/gh-tui: conflict" {
		t.Errorf("Do error = %v, want a wrapped ErrConflict", err)
	}
	wantCached(t, s, []core.Repo{ghTUI, dotfiles}, ghTUI)
	// The entries are restored as they were, fresh, so reads make no call.
	if _, err := s.List(t.Context(), ListQuery{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(t.Context(), ghTUI.Ref); err != nil {
		t.Fatal(err)
	}
	api.wantCalls(t, "star eggzec/gh-tui")
}

func TestStarReconcilesOnSuccess(t *testing.T) {
	api := &fakeAPI{t: t}
	s, _ := seeded(t, api)
	api.star = func(core.RepoRef, bool) error { return nil }

	if err := s.Star(ghTUI.Ref).Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}

	want := toggled(ghTUI, 43)
	wantCached(t, s, []core.Repo{want, dotfiles}, want)

	// Someone else starred it too, so the server has one more.
	server := toggled(ghTUI, 44)
	api.listRepos = func(int, string) (core.Page[core.Repo], error) { return page("c1", server, dotfiles), nil }
	api.getRepo = func(core.RepoRef) (core.Repo, error) { return server, nil }
	if got, err := s.List(t.Context(), ListQuery{}); err != nil || got.Items[0] != server {
		t.Errorf("List = %+v, %v; want the server's count", got, err)
	}
	if got, err := s.Get(t.Context(), ghTUI.Ref); err != nil || got != server {
		t.Errorf("Get = %+v, %v; want the server's count", got, err)
	}
	// Only the entries holding the repo are revalidated.
	if _, err := s.List(t.Context(), ListQuery{After: "c1"}); err != nil {
		t.Fatal(err)
	}
	api.wantCalls(t, "star eggzec/gh-tui", "list 30 ", "get eggzec/gh-tui")
}

func TestUnstar(t *testing.T) {
	api := &fakeAPI{t: t}
	s, _ := seeded(t, api)
	api.star = func(core.RepoRef, bool) error { return core.ErrNotFound }

	op := s.Unstar(dotfiles.Ref)

	// ghTUI isn't starred, so the detail is left alone.
	wantCached(t, s, []core.Repo{ghTUI, toggled(dotfiles, 6)}, ghTUI)
	if err := op.Do(t.Context()); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Do error = %v, want ErrNotFound", err)
	}
	wantCached(t, s, []core.Repo{ghTUI, dotfiles}, ghTUI)
	api.wantCalls(t, "unstar octo-org/dotfiles")
}

func TestStarAlreadyStarred(t *testing.T) {
	api := &fakeAPI{t: t}
	s, _ := seeded(t, api)
	api.star = func(core.RepoRef, bool) error { return nil }

	op := s.Star(dotfiles.Ref)

	wantCached(t, s, []core.Repo{ghTUI, dotfiles}, ghTUI)
	if err := op.Do(t.Context()); err != nil {
		t.Errorf("Do: %v", err)
	}
	api.wantCalls(t, "star octo-org/dotfiles")
}

func TestStarUncached(t *testing.T) {
	var got []core.RepoRef
	api := &fakeAPI{t: t, star: func(ref core.RepoRef, starred bool) error {
		if !starred {
			t.Error("Star sent an unstar")
		}
		got = append(got, ref)
		return nil
	}}
	s := New(api)

	if err := s.Star(ghTUI.Ref).Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !slices.Equal(got, []core.RepoRef{ghTUI.Ref}) {
		t.Errorf("starred %v, want %v", got, ghTUI.Ref)
	}
	if _, ok := s.CachedGet(ghTUI.Ref); ok {
		t.Error("Star cached a repository it never fetched")
	}
}
