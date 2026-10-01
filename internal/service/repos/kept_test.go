package repos

import (
	"errors"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/config"
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
	if _, err := New(api, WithStore(cachetest.Aged(store, time.Hour))).List(t.Context(), ListQuery{}); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestKeptListIsServedStaleThenFetched(t *testing.T) {
	store := keptRepos(t)
	api := &fakeAPI{t: t, listRepos: func(int, string) (core.Page[core.Repo], error) { return mine, nil }}
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	p, err := s.List(t.Context(), ListQuery{})
	if err != nil || !p.Stale || len(p.Items) != 1 {
		t.Fatalf("List in a new session = %+v, %v; want the kept page, stale", p, err)
	}
	if calls := api.Calls(); len(calls) != 0 {
		t.Errorf("calls = %q, want none for the kept page", calls)
	}
	if p, err = s.List(t.Context(), ListQuery{}.again()); err != nil || p.Stale {
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
		{"unreachable", fmt.Errorf("%w: %w", core.ErrOffline, &url.Error{Op: "Post", URL: "https://api.github.com/graphql", Err: errors.New("refused")}), true},
		{"server error", &github.Error{StatusCode: 502}, true},
		{"rate limited", fmt.Errorf("graphql: %w", &core.RateLimitError{Reset: time.Now().Add(time.Hour)}), true},
		{"unauthorized", &github.Error{StatusCode: 401}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := keptRepos(t)
			var mu sync.Mutex
			failure := tt.err
			api := &fakeAPI{t: t, listRepos: func(int, string) (core.Page[core.Repo], error) {
				mu.Lock()
				defer mu.Unlock()
				if failure != nil {
					return core.Page[core.Repo]{}, failure
				}
				return mine, nil
			}}
			s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
			_, _ = s.List(t.Context(), ListQuery{})
			p, err := s.List(t.Context(), ListQuery{}.again())
			limited := errors.Is(tt.err, core.ErrRateLimited)
			if tt.fallback != (err == nil && p.Offline != limited && p.Limited == limited && len(p.Items) == 1) {
				t.Errorf("List = %+v, %v; want fallback %v", p, err, tt.fallback)
			}
			if !tt.fallback {
				if p, _ := New(api, WithStore(cachetest.Aged(store, time.Hour))).List(t.Context(), ListQuery{}); p.Stale {
					t.Error("List after a refusal = stale, want the kept page gone")
				}
				return
			}
			// Once GitHub answers, the page is served unmarked.
			mu.Lock()
			failure = nil
			mu.Unlock()
			if p, err := s.List(t.Context(), ListQuery{}); err != nil || p.Offline || p.Limited || len(p.Items) != 1 {
				t.Errorf("List after the outage = %+v, %v; want the page unmarked", p, err)
			}
		})
	}
}

func TestKeptRepo(t *testing.T) {
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	caps := ghTUI
	caps.Caps = core.RepoCaps{Known: true, Permission: core.PermissionRead, Issues: true}
	api := &fakeAPI{t: t, getRepo: func(core.RepoRef) (core.Repo, error) { return caps, nil }}
	if _, err := New(api, WithStore(store)).Get(t.Context(), ghTUI.Ref); err != nil {
		t.Fatal(err)
	}

	// A new session within its TTL reads the kept repository without a
	// request.
	api = &fakeAPI{t: t}
	if got, err := New(api, WithStore(store)).Get(t.Context(), ghTUI.Ref); err != nil || got != caps {
		t.Errorf("Get in a new session = %+v, %v; want the kept repository", got, err)
	}
	api.wantCalls(t)

	// Once it is older, it is fetched, and served if GitHub can't be
	// reached.
	offline := fmt.Errorf("%w: %w", core.ErrOffline, &url.Error{Op: "Post", URL: "https://api.github.com/graphql", Err: errors.New("refused")})
	api = &fakeAPI{t: t, getRepo: func(core.RepoRef) (core.Repo, error) { return core.Repo{}, offline }}
	if got, err := New(api, WithStore(cachetest.Aged(store, 2*config.Default().Cache.TTL.RepoInfo))).Get(t.Context(), ghTUI.Ref); err != nil || got != caps {
		t.Errorf("Get offline = %+v, %v; want the kept repository", got, err)
	}
	api.wantCalls(t, "get eggzec/gh-tui")

	// A refusal drops it.
	api = &fakeAPI{t: t, getRepo: func(core.RepoRef) (core.Repo, error) { return core.Repo{}, &github.Error{StatusCode: 404} }}
	if _, err := New(api, WithStore(cachetest.Aged(store, 2*config.Default().Cache.TTL.RepoInfo))).Get(t.Context(), ghTUI.Ref); err == nil {
		t.Error("Get after a refusal succeeded")
	}
	api = &fakeAPI{t: t, getRepo: func(core.RepoRef) (core.Repo, error) { return ghTUI, nil }}
	if got, err := New(api, WithStore(store)).Get(t.Context(), ghTUI.Ref); err != nil || got != ghTUI {
		t.Errorf("Get after the refusal = %+v, %v; want it fetched", got, err)
	}
	api.wantCalls(t, "get eggzec/gh-tui")
}
