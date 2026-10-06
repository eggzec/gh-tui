package owners

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
)

// Each page of a list is read once and keyed by its cursor and size, and
// each list of a login by its kind.
func TestPeoplePaging(t *testing.T) {
	api := &fakeAPI{t: t, people: func(list, _ string, _ int, after string) (core.Page[core.Person], error) {
		if after == "" {
			return core.Page[core.Person]{Items: people(list, 2), Next: "c1"}, nil
		}
		return core.Page[core.Person]{Items: people(list+"-more", 1)}, nil
	}}
	s := New(api, WithSizes(Sizes{People: 2}))
	ctx := t.Context()
	first, err := s.People(ctx, PeopleQuery{Login: "Octocat"})
	if err != nil || len(first.Items) != 2 || first.Next != "c1" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	second, err := s.People(ctx, PeopleQuery{Login: "octocat", Cursor: first.Next})
	if err != nil || len(second.Items) != 1 || second.Next != "" {
		t.Fatalf("second page = %+v, %v", second, err)
	}
	for _, q := range []PeopleQuery{{Login: "OCTOCAT"}, {Login: "octocat", Cursor: "c1"}} {
		if _, err := s.People(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.People(ctx, PeopleQuery{Login: "octocat", List: Following}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.People(ctx, PeopleQuery{Login: "octocat", PageSize: 500}); err != nil {
		t.Fatal(err)
	}
	if p, ok := s.CachedPeople(PeopleQuery{Login: "octocat", Cursor: "c1"}); !ok || len(p.Items) != 1 {
		t.Errorf("CachedPeople = %+v, %v; want the second page", p, ok)
	}
	api.wantCalls(t, "followers Octocat 2 ", "followers octocat 2 c1", "following octocat 2 ", "followers octocat 100 ")
}

// Teams and stars page as the lists of people do, with their own sizes.
func TestTeamsAndStarsPaging(t *testing.T) {
	api := &fakeAPI{t: t,
		teams: func(_ string, _ int, after string) (core.Page[core.Team], error) {
			if after == "" {
				return core.Page[core.Team]{Items: []core.Team{{Slug: "a"}}, Next: "t1"}, nil
			}
			return core.Page[core.Team]{Items: []core.Team{{Slug: "b"}}}, nil
		},
		stars: func(_ string, _ int, after string) (core.Page[core.Repo], error) {
			return core.Page[core.Repo]{Items: repos("charm", 1), Next: after + "s"}, nil
		},
	}
	s := New(api, WithSizes(Sizes{Repos: 5, People: 3}))
	ctx := t.Context()
	for range 2 {
		for _, cursor := range []string{"", "t1"} {
			if _, err := s.Teams(ctx, TeamsQuery{Login: "Charm", Cursor: cursor}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Stars(ctx, StarsQuery{Login: "Octocat", Cursor: cursor}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Stars share the repositories' cache, under keys of their own.
	api.userRepos = func(login string, _ core.RepoOrder, _ int, _ string) (core.Page[core.Repo], error) {
		return core.Page[core.Repo]{Items: repos(login, 3)}, nil
	}
	if p, err := s.Repos(ctx, ReposQuery{Owner: "octocat"}); err != nil || len(p.Items) != 3 {
		t.Errorf("Repos = %+v, %v; want the repositories, not the stars", p, err)
	}
	api.wantCalls(t, "teams Charm 3 ", "stars Octocat 5 ", "teams Charm 3 t1", "stars Octocat 5 t1", "user octocat 5 ")
}

// Where GitHub has no GitHub Sponsors, both lists of sponsors fail with
// core.ErrUnsupported, asked about once for the session, and the other
// lists don't.
func TestSponsorsUnsupported(t *testing.T) {
	api := &fakeAPI{t: t, people: func(list, _ string, _ int, _ string) (core.Page[core.Person], error) {
		if list == "sponsors" || list == "sponsoring" {
			return core.Page[core.Person]{}, fmt.Errorf("list %s: %w", list, core.ErrUnsupported)
		}
		return core.Page[core.Person]{Items: people(list, 1)}, nil
	}}
	s := New(api)
	ctx := t.Context()
	if _, err := s.People(ctx, PeopleQuery{Login: "octocat", List: Sponsors}); !errors.Is(err, core.ErrUnsupported) {
		t.Fatalf("Sponsors = %v, want core.ErrUnsupported", err)
	}
	s.Invalidate()
	for _, q := range []PeopleQuery{{Login: "octocat", List: Sponsors, Again: true}, {Login: "charm", List: Sponsoring}} {
		if _, err := s.People(ctx, q); !errors.Is(err, core.ErrUnsupported) {
			t.Errorf("People(%+v) = %v, want core.ErrUnsupported", q, err)
		}
	}
	if _, err := s.People(ctx, PeopleQuery{Login: "octocat", List: Followers}); err != nil {
		t.Errorf("Followers = %v", err)
	}
	api.wantCalls(t, "sponsors octocat 50 ", "followers octocat 50 ")
}

// Teams refused to someone outside the organization fail with
// core.ErrForbidden, unmarked, drop what was kept, and aren't asked about
// again for a while.
func TestTeamsMembersOnly(t *testing.T) {
	teams := ownerReadNamed(t, "teams")
	store := openStore(t)
	keep(t, teams, store)

	membersOnly := fmt.Errorf("list teams of charm: only members see the teams: %w", core.ErrForbidden)
	api := &fakeAPI{t: t}
	teams.fake(api, membersOnly)
	s := New(api, WithStore(cachetest.Aged(store, 7*time.Hour)))
	ctx := t.Context()
	if got, err := teams.read(ctx, s, false); err != nil || !got.stale {
		t.Fatalf("first read = %+v, %v; want the kept page, stale", got, err)
	}
	for range 2 {
		got, err := teams.read(ctx, s, true)
		if !errors.Is(err, core.ErrForbidden) || got.ok || got.offline || got.limited {
			t.Fatalf("read = %+v, %v; want core.ErrForbidden alone", got, err)
		}
	}
	if _, err := s.Teams(ctx, TeamsQuery{Login: "charm", Cursor: "next"}); !errors.Is(err, core.ErrForbidden) {
		t.Errorf("another page = %v, want core.ErrForbidden", err)
	}
	if n := len(api.Calls()); n != 1 {
		t.Errorf("%d calls, want the teams asked about once", n)
	}
	// The organization is there; its other reads still ask.
	api.people = func(string, string, int, string) (core.Page[core.Person], error) {
		return core.Page[core.Person]{Items: people("m", 1)}, nil
	}
	if _, err := s.People(ctx, PeopleQuery{Login: "charm", List: Members}); err != nil {
		t.Errorf("Members = %v", err)
	}
	// A refresh of another page leaves the answer be; one of this page,
	// or of every page, forgets it.
	s.InvalidateLogin("github")
	_, _ = teams.read(ctx, s, true)
	if n := len(api.Calls()); n != 2 {
		t.Errorf("%d calls after another login's InvalidateLogin, want the teams not asked about", n)
	}
	s.InvalidateLogin("Charm")
	_, _ = teams.read(ctx, s, true)
	if n := len(api.Calls()); n != 3 {
		t.Errorf("%d calls after InvalidateLogin, want the teams asked about again", n)
	}
	s.Invalidate()
	_, _ = teams.read(ctx, s, true)
	if n := len(api.Calls()); n != 4 {
		t.Errorf("%d calls after Invalidate, want the teams asked about again", n)
	}

	later := &fakeAPI{t: t}
	teams.fake(later, membersOnly)
	if got, _ := teams.read(ctx, New(later, WithStore(cachetest.Aged(store, 7*time.Hour))), false); got.stale {
		t.Error("read in a later session = stale, want the kept page gone")
	}
}

// ownerReadNamed is the ownerRead called name.
func ownerReadNamed(t *testing.T, name string) ownerRead {
	t.Helper()
	for _, r := range ownerReads {
		if r.name == name {
			return r
		}
	}
	t.Fatalf("no read %q", name)
	return ownerRead{}
}
