package dashboard

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// served is what a read returned, as far as the kept tests care.
type served struct {
	ok, stale, offline bool
}

// keptRead is one of the service's reads, run the same way for each.
type keptRead struct {
	name string
	// fake makes api answer this read with err, or with a value if err is
	// nil.
	fake func(api *fakeAPI, err error)
	read func(context.Context, *Service) (served, error)
}

var keptReads = []keptRead{
	{
		name: "header",
		fake: func(api *fakeAPI, err error) {
			api.header = func() (core.Header, error) { return octocat, err }
		},
		read: func(ctx context.Context, s *Service) (served, error) {
			h, err := s.Header(ctx)
			return served{h.Profile.Login == "octocat", h.Stale, h.Offline}, err
		},
	},
	{
		name: "work",
		fake: func(api *fakeAPI, err error) {
			api.work = func(int) (core.Work, error) { return core.Work{Assigned: core.WorkList{Count: 3}}, err }
		},
		read: func(ctx context.Context, s *Service) (served, error) {
			w, err := s.Work(ctx, WorkQuery{})
			return served{w.Assigned.Count == 3, w.Stale, w.Offline}, err
		},
	},
	{
		name: "contributions",
		fake: func(api *fakeAPI, err error) {
			api.contributions = func() (core.Contributions, error) { return core.Contributions{Total: 42}, err }
		},
		read: func(ctx context.Context, s *Service) (served, error) {
			c, err := s.Contributions(ctx)
			return served{c.Total == 42, c.Stale, c.Offline}, err
		},
	},
	{
		name: "repos",
		fake: func(api *fakeAPI, err error) {
			api.orgRepos = func(login string, _ int, _ string) (core.Page[core.Repo], error) {
				return core.Page[core.Repo]{Items: repos(login, 0, 2)}, err
			}
		},
		read: func(ctx context.Context, s *Service) (served, error) {
			p, err := s.Repos(ctx, ReposQuery{Owner: "charm"})
			return served{len(p.Items) == 2, p.Stale, p.Offline}, err
		},
	},
}

// keep runs r once with store, so that a later session finds it kept.
func keep(t *testing.T, r keptRead, store cache.Store) {
	t.Helper()
	api := &fakeAPI{t: t}
	r.fake(api, nil)
	if _, err := r.read(t.Context(), New(api, WithStore(store))); err != nil {
		t.Fatal(err)
	}
}

func openStore(t *testing.T) *disk.Store {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// A session long after the one that kept an entry paints it at once,
// stale, and the next read fetches it.
func TestKeptIsServedStaleThenFetched(t *testing.T) {
	for _, r := range keptReads {
		t.Run(r.name, func(t *testing.T) {
			store := openStore(t)
			keep(t, r, store)

			api := &fakeAPI{t: t}
			r.fake(api, nil)
			s := New(api, WithStore(cachetest.Aged(store, 7*time.Hour)))
			got, err := r.read(t.Context(), s)
			if err != nil || got != (served{ok: true, stale: true}) {
				t.Fatalf("first read = %+v, %v; want the kept value, stale", got, err)
			}
			api.wantCalls(t)
			if got, err = r.read(t.Context(), s); err != nil || got != (served{ok: true}) {
				t.Fatalf("second read = %+v, %v; want the fetched value", got, err)
			}
			if n := len(api.Calls()); n != 1 {
				t.Errorf("%d calls, want one fetch", n)
			}
		})
	}
}

// An entry kept within its TTL is fresh, as if this session fetched it.
func TestKeptWithinTTLIsFresh(t *testing.T) {
	for _, r := range keptReads {
		if r.name == "work" {
			// Its TTL of a minute is too short to rely on here.
			continue
		}
		t.Run(r.name, func(t *testing.T) {
			store := openStore(t)
			keep(t, r, store)

			api := &fakeAPI{t: t}
			got, err := r.read(t.Context(), New(api, WithStore(cachetest.Aged(store, 5*time.Minute))))
			if err != nil || got != (served{ok: true}) {
				t.Errorf("read = %+v, %v; want the kept value, fresh", got, err)
			}
			api.wantCalls(t)
		})
	}
}

// Without a store nothing is kept, and a new session fetches.
func TestColdStartFetches(t *testing.T) {
	for _, r := range keptReads {
		t.Run(r.name, func(t *testing.T) {
			api := &fakeAPI{t: t}
			r.fake(api, nil)
			got, err := r.read(t.Context(), New(api))
			if err != nil || got != (served{ok: true}) {
				t.Errorf("read = %+v, %v; want the fetched value", got, err)
			}
			if n := len(api.Calls()); n != 1 {
				t.Errorf("%d calls, want one fetch", n)
			}
		})
	}
}

// An outage serves the kept entry marked offline; a refusal drops it.
func TestKeptOffline(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		fallback bool
	}{
		{"unreachable", &url.Error{Op: "Post", URL: "https://api.github.com/graphql", Err: errors.New("refused")}, true},
		{"server error", &github.Error{StatusCode: 502}, true},
		{"unauthorized", &github.Error{StatusCode: 401}, false},
		{"not found", core.ErrNotFound, false},
	}
	for _, r := range keptReads {
		for _, tt := range tests {
			t.Run(r.name+"/"+tt.name, func(t *testing.T) {
				store := openStore(t)
				keep(t, r, store)

				api := &fakeAPI{t: t}
				r.fake(api, tt.err)
				s := New(api, WithStore(cachetest.Aged(store, 7*time.Hour)))
				// The first read serves the kept entry stale, and the second
				// asks GitHub.
				_, _ = r.read(t.Context(), s)
				got, err := r.read(t.Context(), s)
				if tt.fallback != (err == nil && got == served{ok: true, offline: true}) {
					t.Errorf("read = %+v, %v; want fallback %v", got, err, tt.fallback)
				}
				if tt.fallback {
					return
				}
				later := &fakeAPI{t: t}
				r.fake(later, tt.err)
				if got, _ := r.read(t.Context(), New(later, WithStore(cachetest.Aged(store, 7*time.Hour)))); got.stale {
					t.Error("read after a refusal = stale, want the kept entry gone")
				}
			})
		}
	}
}
