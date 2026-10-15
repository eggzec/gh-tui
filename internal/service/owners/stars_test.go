package owners

import (
	"context"
	"errors"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
)

// starAPI is the repositories service's API, which only stars here.
type starAPI struct {
	reposvc.API
	err error
}

func (a starAPI) Star(context.Context, core.RepoRef) error   { return a.err }
func (a starAPI) Unstar(context.Context, core.RepoRef) error { return a.err }

// starsServed reads a page of stars and of repositories of octocat, so that
// both are cached fresh.
func starsServed(t *testing.T, s *Service, api *fakeAPI) {
	t.Helper()
	if _, err := s.Stars(t.Context(), StarsQuery{Login: "octocat"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Repos(t.Context(), ReposQuery{Owner: "octocat", Kind: core.OwnerUser}); err != nil {
		t.Fatal(err)
	}
	api.wantCalls(t, "stars octocat 30 ", "user octocat 30 ")
}

// A star the viewer gave or took back makes the page of stars that was kept
// on disk stale too, so a later session shows it at once and reads it again,
// rather than taking it for fresh within its TTL.
func TestStarChangeMarksKeptStarsStale(t *testing.T) {
	ref := core.RepoRef{Owner: "cli", Name: "cli"}
	for _, tc := range []struct {
		name string
		do   func(*reposvc.Service) error
		// stale is whether the next session is served the page as stale.
		stale bool
	}{
		{"unchanged", func(*reposvc.Service) error { return nil }, false},
		{"star", func(r *reposvc.Service) error { return r.Star(ref).Do(t.Context()) }, true},
		{"unstar", func(r *reposvc.Service) error { return r.Unstar(ref).Do(t.Context()) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{
				t: t,
				stars: func(string, int, string) (core.Page[core.Repo], error) {
					return core.Page[core.Repo]{Items: repos("cli", 1)}, nil
				},
				userRepos: func(string, core.RepoOrder, int, string) (core.Page[core.Repo], error) {
					return core.Page[core.Repo]{}, nil
				},
			}
			store := openStore(t)
			s := New(api, WithStore(store))
			starsServed(t, s, api)
			if err := tc.do(reposvc.New(starAPI{}, reposvc.WithStars(s))); err != nil {
				t.Fatal(err)
			}

			next := New(api, WithStore(store))
			p, err := next.Stars(t.Context(), StarsQuery{Login: "octocat"})
			if err != nil || len(p.Items) != 1 || p.Stale != tc.stale {
				t.Errorf("Stars in the next session = %d items, stale %t, %v; want 1 item, stale %t", len(p.Items), p.Stale, err, tc.stale)
			}
			// A kept page of repositories is not one of stars.
			if r, err := next.Repos(t.Context(), ReposQuery{Owner: "octocat", Kind: core.OwnerUser}); err != nil || r.Stale {
				t.Errorf("Repos in the next session = stale %t, %v; want fresh", r.Stale, err)
			}
			// The next session asks for nothing: the stale page is served at once.
			api.wantCalls(t, "stars octocat 30 ", "user octocat 30 ")
		})
	}
}

func TestStarChangeMarksStarsStale(t *testing.T) {
	ref := core.RepoRef{Owner: "cli", Name: "cli"}
	for _, tc := range []struct {
		name string
		do   func(*reposvc.Service) error
		err  error
		// stale is whether the page of stars is read again afterwards.
		stale bool
	}{
		{"star", func(r *reposvc.Service) error { return r.Star(ref).Do(t.Context()) }, nil, true},
		{"unstar", func(r *reposvc.Service) error { return r.Unstar(ref).Do(t.Context()) }, nil, true},
		{"failed star", func(r *reposvc.Service) error { return r.Star(ref).Do(t.Context()) }, core.ErrConflict, false},
		{"failed unstar", func(r *reposvc.Service) error { return r.Unstar(ref).Do(t.Context()) }, core.ErrConflict, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{
				t: t,
				stars: func(string, int, string) (core.Page[core.Repo], error) {
					return core.Page[core.Repo]{Items: repos("cli", 1)}, nil
				},
				userRepos: func(string, core.RepoOrder, int, string) (core.Page[core.Repo], error) {
					return core.Page[core.Repo]{}, nil
				},
			}
			s := New(api)
			starsServed(t, s, api)
			r := reposvc.New(starAPI{err: tc.err}, reposvc.WithStars(s))

			if err := tc.do(r); !errors.Is(err, tc.err) {
				t.Fatalf("Do = %v, want %v", err, tc.err)
			}

			if got := !s.FreshStars(StarsQuery{Login: "octocat"}); got != tc.stale {
				t.Errorf("stars page stale = %t, want %t", got, tc.stale)
			}
			if _, ok := s.CachedStars(StarsQuery{Login: "octocat"}); !ok {
				t.Error("the page of stars was dropped, want it kept to show")
			}
			// Only the pages of stars are marked, not the repositories.
			if !s.FreshRepos(ReposQuery{Owner: "octocat", Kind: core.OwnerUser}) {
				t.Error("the page of repositories is stale, want it left alone")
			}
		})
	}
}
