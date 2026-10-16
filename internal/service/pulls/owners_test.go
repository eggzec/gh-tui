package pulls

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/codeowners"
	"github.com/eggzec/gh-tui/internal/core"
)

const baseSHA = "3333333333333333333333333333333333333333"

func (f *fakeAPI) GetCodeowners(ctx context.Context, repo core.RepoRef, base string) (core.CodeownersFile, error) {
	f.called("codeowners")
	return f.codeowners(ctx, repo, base)
}

func (f *fakeAPI) ListTeamMembers(ctx context.Context, org, slug string) (core.Page[core.Person], error) {
	f.called("teammembers")
	return f.teamMembers(ctx, org, slug)
}

var ownersQ = OwnersQuery{Repo: repo, Base: baseSHA}

func TestOwnersParsedAndKept(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{codeowners: func(_ context.Context, r core.RepoRef, base string) (core.CodeownersFile, error) {
			if r != repo || (base != baseSHA && base != oldHead) {
				t.Errorf("read %s at %s", r, base)
			}
			return core.CodeownersFile{Path: ".github/CODEOWNERS", Text: "*.go @gophers\n/docs/ @octo/docs\nbad !line\n"}, nil
		}}
		s := New(api, WithTTL(time.Minute))
		if _, ok := s.CachedOwners(ownersQ); ok {
			t.Fatal("CachedOwners before a read = true")
		}
		got, err := s.Owners(t.Context(), ownersQ)
		if err != nil {
			t.Fatalf("Owners: %v", err)
		}
		want := []codeowners.Owner{{Kind: codeowners.OwnerUser, Name: "gophers"}}
		if got.Path != ".github/CODEOWNERS" || len(got.Rules) != 2 || len(got.Invalid) != 1 || !slices.Equal(got.OwnersOf("a/b.go"), want) {
			t.Errorf("owners = %+v, want the parsed file", got)
		}
		// The file of a commit never changes, however long ago it was read.
		time.Sleep(48 * time.Hour)
		if c, ok := s.CachedOwners(ownersQ); !ok || c.Path != got.Path {
			t.Errorf("CachedOwners after two days = %+v, %v; want the file", c, ok)
		}
		if _, err := s.Owners(t.Context(), ownersQ); err != nil || api.count("codeowners") != 1 {
			t.Errorf("a second read: err %v, %d requests; want one request in all", err, api.count("codeowners"))
		}
		// Another commit is another file.
		other := OwnersQuery{Repo: repo, Base: oldHead}
		if _, err := s.Owners(t.Context(), other); err != nil || api.count("codeowners") != 2 {
			t.Errorf("another base: err %v, %d requests; want 2", err, api.count("codeowners"))
		}
	})
}

func TestOwnersWithoutFile(t *testing.T) {
	api := &fakeAPI{codeowners: func(context.Context, core.RepoRef, string) (core.CodeownersFile, error) {
		return core.CodeownersFile{}, nil
	}}
	got, err := New(api).Owners(t.Context(), ownersQ)
	if err != nil || got.Path != "" || len(got.Rules) != 0 || got.OwnersOf("a.go") != nil {
		t.Errorf("Owners = %+v, %v; want an empty file", got, err)
	}
}

func TestOwnersErrors(t *testing.T) {
	boom := errors.New("boom")
	api := &fakeAPI{codeowners: func(context.Context, core.RepoRef, string) (core.CodeownersFile, error) {
		return core.CodeownersFile{}, boom
	}}
	s := New(api)
	if _, err := s.Owners(t.Context(), ownersQ); !errors.Is(err, boom) {
		t.Errorf("Owners error = %v, want boom", err)
	}
	// A failed read isn't kept.
	if _, ok := s.CachedOwners(ownersQ); ok {
		t.Error("a failed read was cached")
	}
	if _, err := s.Owners(t.Context(), OwnersQuery{Repo: repo}); !errors.Is(err, errNoBase) || api.count("codeowners") != 1 {
		t.Errorf("a read with no base: err %v, %d requests; want errNoBase and no request", err, api.count("codeowners"))
	}
}

func TestOwnersKeptInStore(t *testing.T) {
	store := openStore(t)
	api := &fakeAPI{codeowners: func(context.Context, core.RepoRef, string) (core.CodeownersFile, error) {
		return core.CodeownersFile{Path: "CODEOWNERS", Text: "*.go @gophers\n"}, nil
	}}
	if _, err := New(api, WithStore(store)).Owners(t.Context(), ownersQ); err != nil {
		t.Fatalf("Owners: %v", err)
	}
	// A new session starts with nothing in memory, and finds the file kept.
	next := &fakeAPI{}
	got, err := New(next, WithStore(store)).Owners(t.Context(), ownersQ)
	if err != nil || got.Path != "CODEOWNERS" || len(got.OwnersOf("a.go")) != 1 {
		t.Errorf("Owners from the store = %+v, %v; want the parsed file", got, err)
	}
	if n := next.count("codeowners"); n != 0 {
		t.Errorf("requests = %d, want none for a kept file", n)
	}
}

func TestOwnersKeptServedOffline(t *testing.T) {
	store := openStore(t)
	ok := &fakeAPI{codeowners: func(context.Context, core.RepoRef, string) (core.CodeownersFile, error) {
		return core.CodeownersFile{Path: "docs/CODEOWNERS", Text: "/docs/ @octo/docs\n"}, nil
	}}
	if _, err := New(ok, WithStore(store)).Owners(t.Context(), ownersQ); err != nil {
		t.Fatalf("Owners: %v", err)
	}
	down := &fakeAPI{codeowners: func(context.Context, core.RepoRef, string) (core.CodeownersFile, error) {
		return core.CodeownersFile{}, core.ErrOffline
	}}
	got, err := New(down, WithStore(store)).Owners(t.Context(), ownersQ)
	if err != nil || got.Path != "docs/CODEOWNERS" || len(got.Rules) != 1 {
		t.Errorf("Owners offline = %+v, %v; want the kept file", got, err)
	}
	// A commit that has no file is kept too, so that it isn't asked again.
	none := OwnersQuery{Repo: repo, Base: oldHead}
	empty := &fakeAPI{codeowners: func(context.Context, core.RepoRef, string) (core.CodeownersFile, error) {
		return core.CodeownersFile{}, nil
	}}
	if _, err := New(empty, WithStore(store)).Owners(t.Context(), none); err != nil {
		t.Fatalf("Owners without file: %v", err)
	}
	if _, err := New(down, WithStore(store)).Owners(t.Context(), none); err != nil {
		t.Errorf("Owners offline for a commit without a file: %v", err)
	}
}

var teamQ = TeamQuery{Org: "octo", Slug: "docs"}

func people(logins ...string) []core.Person {
	out := make([]core.Person, 0, len(logins))
	for _, l := range logins {
		out = append(out, core.Person{Login: l})
	}
	return out
}

func TestTeamMembersTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{teamMembers: func(_ context.Context, org, slug string) (core.Page[core.Person], error) {
			if org != "octo" || slug != "docs" {
				t.Errorf("read team %s/%s", org, slug)
			}
			return core.Page[core.Person]{Items: people("mona", "hubot"), Truncated: true}, nil
		}}
		s := New(api, WithTTL(time.Minute), WithAccess(tokenWith(classic("read:org"), true)))
		for range 2 {
			got, err := s.TeamMembers(t.Context(), teamQ)
			if err != nil || got.Unavailable || !got.Truncated || !slices.Equal(got.Members, people("mona", "hubot")) {
				t.Fatalf("TeamMembers = %+v, %v; want mona and hubot", got, err)
			}
		}
		if api.count("teammembers") != 1 {
			t.Errorf("requests = %d, want 1: a fresh list is a hit", api.count("teammembers"))
		}
		if c, ok := s.CachedTeamMembers(TeamQuery{Org: "OCTO", Slug: "Docs"}); !ok || len(c.Members) != 2 {
			t.Errorf("CachedTeamMembers = %+v, %v; want the members, whatever the case", c, ok)
		}
		time.Sleep(2 * time.Minute)
		if _, err := s.TeamMembers(t.Context(), teamQ); err != nil || api.count("teammembers") != 2 {
			t.Errorf("after the TTL: err %v, %d requests; want 2", err, api.count("teammembers"))
		}
	})
}

func TestTeamMembersWithoutScope(t *testing.T) {
	api := &fakeAPI{teamMembers: func(context.Context, string, string) (core.Page[core.Person], error) {
		t.Error("the members were read without read:org")
		return core.Page[core.Person]{}, nil
	}}
	s := New(api, WithAccess(tokenWith(classic("repo"), true)))
	got, err := s.TeamMembers(t.Context(), teamQ)
	if err != nil || !got.Unavailable || len(got.Members) != 0 {
		t.Errorf("TeamMembers = %+v, %v; want Unavailable and no error", got, err)
	}
}

func TestTeamMembersScopeKnown(t *testing.T) {
	// admin:org covers read:org, and a token whose scopes are unknown may try.
	for _, a := range []core.Access{classic("admin:org"), {}} {
		api := &fakeAPI{teamMembers: func(context.Context, string, string) (core.Page[core.Person], error) {
			return core.Page[core.Person]{Items: people("mona")}, nil
		}}
		got, err := New(api, WithAccess(tokenWith(a, true))).TeamMembers(t.Context(), teamQ)
		if err != nil || got.Unavailable || len(got.Members) != 1 {
			t.Errorf("TeamMembers with %+v = %+v, %v; want mona", a, got, err)
		}
	}
}

func TestTeamMembersNotFound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{teamMembers: func(context.Context, string, string) (core.Page[core.Person], error) {
			return core.Page[core.Person]{}, errors.Join(core.ErrNotFound)
		}}
		s := New(api, WithTTL(time.Minute))
		for range 2 {
			got, err := s.TeamMembers(t.Context(), teamQ)
			if err != nil || !got.Unavailable || len(got.Members) != 0 {
				t.Fatalf("TeamMembers = %+v, %v; want Unavailable and no error", got, err)
			}
		}
		if api.count("teammembers") != 1 {
			t.Errorf("requests = %d, want 1: the 404 is cached for the TTL", api.count("teammembers"))
		}
	})
}

func TestTeamMembersForbidden(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{teamMembers: func(context.Context, string, string) (core.Page[core.Person], error) {
			return core.Page[core.Person]{}, fmt.Errorf("resource not accessible: %w", core.ErrForbidden)
		}}
		s := New(api, WithTTL(time.Minute))
		for range 2 {
			got, err := s.TeamMembers(t.Context(), teamQ)
			if err != nil || !got.Unavailable || len(got.Members) != 0 {
				t.Fatalf("TeamMembers = %+v, %v; want Unavailable and no error", got, err)
			}
		}
		if api.count("teammembers") != 1 {
			t.Errorf("requests = %d, want 1: the 403 is cached for the TTL", api.count("teammembers"))
		}
	})
}

func TestTeamMembersOffline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fail := false
		api := &fakeAPI{teamMembers: func(context.Context, string, string) (core.Page[core.Person], error) {
			if fail {
				return core.Page[core.Person]{}, core.ErrOffline
			}
			return core.Page[core.Person]{Items: people("mona")}, nil
		}}
		s := New(api, WithTTL(time.Minute))
		if _, err := s.TeamMembers(t.Context(), teamQ); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Minute)
		fail = true
		got, err := s.TeamMembers(t.Context(), teamQ)
		if err != nil || !got.Offline || len(got.Members) != 1 {
			t.Errorf("TeamMembers offline = %+v, %v; want the stale members, Offline", got, err)
		}
		fail = false
		got, err = s.TeamMembers(t.Context(), teamQ)
		if err != nil || got.Offline {
			t.Errorf("after GitHub answers = %+v, %v; want fresh members", got, err)
		}
	})
}

func TestTeamMembersError(t *testing.T) {
	boom := errors.New("boom")
	api := &fakeAPI{teamMembers: func(context.Context, string, string) (core.Page[core.Person], error) {
		return core.Page[core.Person]{}, boom
	}}
	if _, err := New(api).TeamMembers(t.Context(), teamQ); !errors.Is(err, boom) {
		t.Errorf("TeamMembers error = %v, want boom", err)
	}
}
