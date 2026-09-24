package repos

import (
	"errors"
	"net/url"
	"testing"

	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var mine = core.Page[core.Repo]{Items: []core.Repo{{Ref: core.RepoRef{Owner: "octocat", Name: "hello"}}}}

func keptRepos(t *testing.T) *disk.Store {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := &fakeAPI{t: t, listRepos: func(int, string) (core.Page[core.Repo], error) { return mine, nil }}
	if _, err := New(api, WithStore(store)).List(t.Context(), ListQuery{}); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestKeptListIsServedStaleThenFetched(t *testing.T) {
	store := keptRepos(t)
	api := &fakeAPI{t: t, listRepos: func(int, string) (core.Page[core.Repo], error) { return mine, nil }}
	s := New(api, WithStore(store))
	p, err := s.List(t.Context(), ListQuery{})
	if err != nil || !p.Stale || len(p.Items) != 1 {
		t.Fatalf("List in a new session = %+v, %v; want the kept page, stale", p, err)
	}
	if calls := api.Calls(); len(calls) != 0 {
		t.Errorf("calls = %q, want none for the kept page", calls)
	}
	if p, err = s.List(t.Context(), ListQuery{}); err != nil || p.Stale {
		t.Fatalf("second List = %+v, %v; want the page fetched", p, err)
	}
	if calls := api.Calls(); len(calls) != 1 {
		t.Errorf("calls = %q, want one fetch", calls)
	}
}

func TestKeptListOffline(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		fallback bool
	}{
		{"unreachable", &url.Error{Op: "Post", URL: "https://api.github.com/graphql", Err: errors.New("refused")}, true},
		{"server error", &github.Error{StatusCode: 502}, true},
		{"unauthorized", &github.Error{StatusCode: 401}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := keptRepos(t)
			api := &fakeAPI{t: t, listRepos: func(int, string) (core.Page[core.Repo], error) { return core.Page[core.Repo]{}, tt.err }}
			s := New(api, WithStore(store))
			_, _ = s.List(t.Context(), ListQuery{})
			p, err := s.List(t.Context(), ListQuery{})
			if tt.fallback != (err == nil && p.Offline && len(p.Items) == 1) {
				t.Errorf("List = %+v, %v; want fallback %v", p, err, tt.fallback)
			}
			if !tt.fallback {
				if p, _ := New(api, WithStore(store)).List(t.Context(), ListQuery{}); p.Stale {
					t.Error("List after a refusal = stale, want the kept page gone")
				}
			}
		})
	}
}
