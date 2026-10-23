package repos

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/cache/disk"
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
	for _, q := range []ListQuery{{}, {Cursor: "c1"}} {
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
	if got, _ := s.CachedList(ListQuery{}); !equalPage(got, page(got.Next, first...)) || got.Next != "c1" {
		t.Errorf("first page = %+v, want %+v", got, first)
	}
	if got, _ := s.CachedList(ListQuery{Cursor: "c1"}); !equalPage(got, page("", other)) {
		t.Errorf("second page = %+v, want it untouched", got)
	}
	if got, _ := s.CachedGet(ghTUI.Ref); !reflect.DeepEqual(got, detail) {
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
	if !reflect.DeepEqual(items[0], ghTUI) {
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
	if got, err := s.List(t.Context(), ListQuery{}); err != nil || !reflect.DeepEqual(got.Items[0], server) {
		t.Errorf("List = %+v, %v; want the server's count", got, err)
	}
	if got, err := s.Get(t.Context(), ghTUI.Ref); err != nil || !reflect.DeepEqual(got, server) {
		t.Errorf("Get = %+v, %v; want the server's count", got, err)
	}
	// Only the entries holding the repo are revalidated.
	if _, err := s.List(t.Context(), ListQuery{Cursor: "c1"}); err != nil {
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

// TestStarFailureLeavesTheKeptCopyAsItWas checks that a star or unstar that
// fails leaves what is kept on disk as GitHub last sent it, while the change
// is pending and after, so a new session never starts from a guessed star.
func TestStarFailureLeavesTheKeptCopyAsItWas(t *testing.T) {
	for _, tt := range []struct {
		name string
		do   func(*Service) error
		repo core.Repo
	}{
		{"star", func(s *Service) error { return s.Star(ghTUI.Ref).Do(t.Context()) }, ghTUI},
		{"unstar", func(s *Service) error { return s.Unstar(dotfiles.Ref).Do(t.Context()) }, dotfiles},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, err := disk.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			list := func(int, string) (core.Page[core.Repo], error) { return page("", ghTUI, dotfiles), nil }
			get := func(ref core.RepoRef) (core.Repo, error) {
				if sameRepo(ref, dotfiles.Ref) {
					return dotfiles, nil
				}
				return ghTUI, nil
			}
			api := &fakeAPI{t: t, listRepos: list, getRepo: get}
			s := New(api, WithStore(store))
			if _, err := s.List(t.Context(), ListQuery{}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Get(t.Context(), tt.repo.Ref); err != nil {
				t.Fatal(err)
			}

			// A new session reads the kept copy: the repository fresh, and the
			// list page stale. Neither shows a star that was never confirmed.
			kept := func(when string) {
				t.Helper()
				api := &fakeAPI{t: t, listRepos: list, getRepo: get}
				ns := New(api, WithStore(store))
				if got, err := ns.Get(t.Context(), tt.repo.Ref); err != nil || !reflect.DeepEqual(got, tt.repo) {
					t.Errorf("kept repository %s = %+v, %v; want %+v", when, got, err, tt.repo)
				}
				got, err := ns.List(t.Context(), ListQuery{})
				if err != nil || !slices.ContainsFunc(got.Items, func(r core.Repo) bool { return reflect.DeepEqual(r, tt.repo) }) {
					t.Errorf("kept list %s = %+v, %v; want it to hold %+v", when, got.Items, err, tt.repo)
				}
				api.wantCalls(t)
			}

			api.star = func(core.RepoRef, bool) error {
				kept("while the change is sent")
				return core.ErrForbidden
			}
			if err := tt.do(s); !errors.Is(err, core.ErrForbidden) {
				t.Fatalf("Do error = %v, want ErrForbidden", err)
			}
			kept("after the failure")
			if got, _ := s.CachedGet(tt.repo.Ref); !reflect.DeepEqual(got, tt.repo) {
				t.Errorf("cached repository = %+v, want %+v", got, tt.repo)
			}
		})
	}
}

// TestStarMarksTheKeptCopyStale checks that a confirmed star or unstar makes
// what is kept on disk stale, so a new session doesn't take the repository,
// which has the old count, for fresh within its TTL.
func TestStarMarksTheKeptCopyStale(t *testing.T) {
	for _, tt := range []struct {
		name string
		do   func(*Service) error
		repo core.Repo
	}{
		{"star", func(s *Service) error { return s.Star(ghTUI.Ref).Do(t.Context()) }, ghTUI},
		{"unstar", func(s *Service) error { return s.Unstar(dotfiles.Ref).Do(t.Context()) }, dotfiles},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, err := disk.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			list := func(int, string) (core.Page[core.Repo], error) { return page("", ghTUI, dotfiles), nil }
			get := func(ref core.RepoRef) (core.Repo, error) {
				if sameRepo(ref, dotfiles.Ref) {
					return dotfiles, nil
				}
				return ghTUI, nil
			}
			api := &fakeAPI{t: t, listRepos: list, getRepo: get, star: func(core.RepoRef, bool) error { return nil }}
			s := New(api, WithStore(store))
			if _, err := s.List(t.Context(), ListQuery{}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Get(t.Context(), tt.repo.Ref); err != nil {
				t.Fatal(err)
			}
			if err := tt.do(s); err != nil {
				t.Fatal(err)
			}

			api = &fakeAPI{t: t, listRepos: list, getRepo: get}
			ns := New(api, WithStore(store))
			if got, err := ns.List(t.Context(), ListQuery{}); err != nil || !got.Stale {
				t.Errorf("kept list = stale %t, %v; want it stale", got.Stale, err)
			}
			if _, err := ns.Get(t.Context(), tt.repo.Ref); err != nil {
				t.Fatal(err)
			}
			api.wantCalls(t, "get "+tt.repo.Ref.String())
		})
	}
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
