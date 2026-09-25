package pulls

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// pathAPI records which read answered each list: the repository's list or
// a search, and what it was asked.
type pathAPI struct {
	fakeAPI
	mu       sync.Mutex
	filters  []github.PullFilter
	searches []string
}

func newPathAPI() *pathAPI {
	a := &pathAPI{}
	a.list = listing
	a.filter = func(_ context.Context, _ core.RepoRef, f github.PullFilter, _ string, _ int) (core.Page[core.PullRequest], error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.filters = append(a.filters, f)
		return core.Page[core.PullRequest]{Items: []core.PullRequest{openPull(7)}}, nil
	}
	a.search = func(_ context.Context, q, _ string, _ int) (core.Page[core.PullRequest], error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.searches = append(a.searches, q)
		return core.Page[core.PullRequest]{Items: []core.PullRequest{openPull(8)}}, nil
	}
	return a
}

func TestListPicksThePath(t *testing.T) {
	tests := []struct {
		name   string
		state  core.State
		filter string
		// Exactly one of these is set.
		list   *github.PullFilter
		search string
	}{
		{name: "one label", state: core.StateOpen, filter: "label:bug",
			list: &github.PullFilter{State: core.StateOpen, Labels: []string{"bug"}}},
		{name: "any of two labels", state: core.StateMerged, filter: `label:bug,"good first issue"`,
			list: &github.PullFilter{State: core.StateMerged, Labels: []string{"bug", "good first issue"}}},
		{name: "branches and sort", filter: "base:main head:feat/x sort:created-asc",
			list: &github.PullFilter{Base: "main", Head: "feat/x", Sort: "created", Asc: true}},
		{name: "author", state: core.StateOpen, filter: "author:@me",
			search: "repo:eggzec/gh-tui is:pr is:open author:@me sort:updated-desc"},
		{name: "both of two labels", state: core.StateClosed, filter: "label:bug label:ui",
			search: "repo:eggzec/gh-tui is:pr is:closed is:unmerged label:bug label:ui sort:updated-desc"},
		{name: "no drafts, sorted", state: core.StateMerged, filter: "-is:draft sort:comments-desc",
			search: "repo:eggzec/gh-tui is:pr is:merged -is:draft sort:comments-desc"},
		{name: "review and words", filter: "review:approved crash",
			search: "repo:eggzec/gh-tui is:pr review:approved crash sort:updated-desc"},
		{name: "unknown sort", filter: "sort:reactions-+1",
			search: "repo:eggzec/gh-tui is:pr sort:reactions-+1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newPathAPI()
			s := New(api)
			p, err := s.List(t.Context(), ListQuery{Repo: repo, State: tt.state, Filter: tt.filter})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if api.count("list") != 0 {
				t.Error("a filtered list read the unfiltered one")
			}
			if tt.list != nil {
				if len(api.filters) != 1 || !reflect.DeepEqual(api.filters[0], *tt.list) || len(api.searches) != 0 {
					t.Errorf("filters %+v, searches %q; want the list with %+v", api.filters, api.searches, *tt.list)
				}
				if p.Items[0].Number != 7 {
					t.Errorf("page = %+v, want the list's", p)
				}
				return
			}
			if len(api.searches) != 1 || api.searches[0] != tt.search || len(api.filters) != 0 {
				t.Errorf("searches %q, filters %+v; want the search %q", api.searches, api.filters, tt.search)
			}
		})
	}
}

func TestListCachesEachFilter(t *testing.T) {
	api := newPathAPI()
	s := New(api)
	q := ListQuery{Repo: repo, State: core.StateOpen, Filter: "author:@me"}
	other := q
	other.Filter = "author:octocat"
	for _, q := range []ListQuery{q, q, other, {Repo: repo, State: core.StateOpen}} {
		if _, err := s.List(t.Context(), q); err != nil {
			t.Fatalf("List: %v", err)
		}
	}
	if n, l := api.count("search"), api.count("list"); n != 2 || l != 1 {
		t.Errorf("searched %d times and listed %d; want each filter read once, and the plain list apart", n, l)
	}
	if p, ok := s.CachedList(q); !ok || p.Items[0].Number != 8 {
		t.Errorf("CachedList = %+v, %v; want the filtered page without a request", p, ok)
	}
	if p, ok := s.CachedList(ListQuery{Repo: repo, State: core.StateOpen}); !ok || p.Items[0].Number != 1 {
		t.Errorf("CachedList of the plain list = %+v, %v; want its own page", p, ok)
	}
	// A refresh reaches the filtered lists too.
	s.Invalidate(repo)
	if _, err := s.List(t.Context(), q); err != nil {
		t.Fatalf("List: %v", err)
	}
	if n := api.count("search"); n != 3 {
		t.Errorf("searched %d times after Invalidate, want 3", n)
	}
}

func TestFilteredListsAreNotKept(t *testing.T) {
	store := cachetest.Aged(openStore(t), time.Hour)
	filtered := ListQuery{Repo: repo, State: core.StateOpen, Filter: "author:@me"}
	s := New(newPathAPI(), WithStore(store))
	list(t, s, openList)
	list(t, s, filtered)

	api := newPathAPI()
	s = New(api, WithStore(store))
	if p, err := s.List(t.Context(), openList); err != nil || !p.Stale {
		t.Errorf("plain list in a new session = %+v, %v; want the kept page", p, err)
	}
	if p, err := s.List(t.Context(), filtered); err != nil || p.Stale || api.count("search") != 1 {
		t.Errorf("filtered list in a new session = %+v, %v, %d searches; want it read again", p, err, api.count("search"))
	}
}
