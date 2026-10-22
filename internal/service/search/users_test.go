package search

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/eggzec/gh-tui/internal/core"
)

func (f *fakeAPI) SearchUsers(_ context.Context, query, cursor string, perPage int) (core.SearchPage[core.SearchUser], error) {
	f.record(fmt.Sprintf("users %q %d %s", query, perPage, cursor))
	if f.users == nil {
		f.t.Error("unexpected SearchUsers")
		return core.SearchPage[core.SearchUser]{}, errors.New("unexpected call")
	}
	return f.users(query, cursor, perPage)
}

func usersAPI(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{t: t, users: func(string, string, int) (core.SearchPage[core.SearchUser], error) {
		return core.SearchPage[core.SearchUser]{
			Items: []core.SearchUser{{Login: "muesli"}},
			Total: 1,
		}, nil
	}}
}

func TestUsersCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := usersAPI(t)
		s := New(api)
		q := UsersQuery{Text: "  muesli ", Kind: core.SearchUsers}
		if _, ok := s.CachedUsers(q); ok {
			t.Error("CachedUsers hit before any search")
		}
		got, err := s.Users(t.Context(), q)
		if err != nil || got.Total != 1 || len(got.Items) != 1 {
			t.Fatalf("Users = %+v, %v", got, err)
		}
		if _, err := s.Users(t.Context(), UsersQuery{Text: "MUESLI", Kind: core.SearchUsers}); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.CachedUsers(q); !ok {
			t.Error("CachedUsers missed after a search")
		}
		api.wantCalls(t, `users "muesli type:user" 20 `)
	})
}

func TestUsersError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		boom := errors.New("boom")
		api := &fakeAPI{t: t, users: func(string, string, int) (core.SearchPage[core.SearchUser], error) {
			return core.SearchPage[core.SearchUser]{}, boom
		}}
		s := New(api)
		q := UsersQuery{Text: "muesli", Kind: core.SearchUsers}
		for range 2 {
			if _, err := s.Users(t.Context(), q); !errors.Is(err, boom) {
				t.Fatalf("Users error = %v, want it to wrap boom", err)
			}
		}
		if _, ok := s.CachedUsers(q); ok {
			t.Error("a failed search was cached")
		}
		api.wantCalls(t, `users "muesli type:user" 20 `, `users "muesli type:user" 20 `)
	})
}

func TestUsersAllSendsBareText(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := usersAPI(t)
		s := New(api)
		all := UsersQuery{Text: "muesli", Kind: core.SearchAll}
		if _, err := s.Users(t.Context(), all); err != nil {
			t.Fatal(err)
		}
		for _, k := range []core.SearchKind{core.SearchUsers, core.SearchOrgs} {
			if _, ok := s.CachedUsers(UsersQuery{Text: "muesli", Kind: k}); ok {
				t.Errorf("kind %v shares the cache of all", k)
			}
		}
		if _, ok := s.CachedUsers(all); !ok {
			t.Error("CachedUsers of all missed after a search")
		}
		api.wantCalls(t, `users "muesli" 20 `)
	})
}

func TestUsersEmptyText(t *testing.T) {
	api := &fakeAPI{t: t}
	s := New(api)
	got, err := s.Users(t.Context(), UsersQuery{Text: "   ", Kind: core.SearchOrgs})
	if err != nil || len(got.Items) != 0 {
		t.Errorf("Users = %+v, %v; want an empty page", got, err)
	}
	if _, ok := s.CachedUsers(UsersQuery{}); !ok {
		t.Error("CachedUsers of no text missed")
	}
	api.wantCalls(t)
}

func TestUsersCursorAndPageSizeKeyPages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := usersAPI(t)
		s := New(api)
		first := UsersQuery{Text: "charm", Kind: core.SearchUsers}
		for _, q := range []UsersQuery{
			first,
			{Text: "charm", Kind: core.SearchUsers, Cursor: "next"},
			{Text: "charm", Kind: core.SearchUsers, PageSize: 5},
			first,
		} {
			if _, err := s.Users(t.Context(), q); err != nil {
				t.Fatal(err)
			}
		}
		api.wantCalls(t, `users "charm type:user" 20 `, `users "charm type:user" 20 next`, `users "charm type:user" 5 `)
	})
}

func TestUsersInvalidate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := usersAPI(t)
		s := New(api)
		q := UsersQuery{Text: "charm", Kind: core.SearchUsers}
		if _, err := s.Users(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		s.Invalidate()
		if _, err := s.Users(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		api.wantCalls(t, `users "charm type:user" 20 `, `users "charm type:user" 20 `)
	})
}

func TestUsersKeysDifferByKind(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := usersAPI(t)
		s := New(api)
		for _, q := range []UsersQuery{
			{Text: "charm", Kind: core.SearchUsers},
			{Text: "charm", Kind: core.SearchOrgs},
			{Text: "charm type:org", Kind: core.SearchOrgs},
			{Text: "charm type:org", Kind: core.SearchUsers},
		} {
			if _, err := s.Users(t.Context(), q); err != nil {
				t.Fatal(err)
			}
		}
		// A qualifier the user typed is never added to, and so shares the
		// page of the same query.
		api.wantCalls(t, `users "charm type:user" 20 `, `users "charm type:org" 20 `)
	})
}
