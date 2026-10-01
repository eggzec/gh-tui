package dashboard

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

func TestHeaderCachesUntilInvalidated(t *testing.T) {
	api := &fakeAPI{t: t, header: func() (core.Header, error) { return octocat, nil }}
	s := New(api)
	if _, ok := s.CachedHeader(); ok {
		t.Error("CachedHeader before a read = hit, want a miss")
	}
	for range 2 {
		h, err := s.Header(t.Context(), HeaderQuery{})
		if err != nil || h.Profile.Login != "octocat" || h.Stale || h.Offline {
			t.Fatalf("Header = %+v, %v; want octocat", h, err)
		}
	}
	api.wantCalls(t, "header")
	if h, ok := s.CachedHeader(); !ok || h.Profile.Login != "octocat" {
		t.Errorf("CachedHeader = %+v, %v; want octocat", h, ok)
	}

	s.Invalidate()
	if _, ok := s.CachedHeader(); !ok {
		t.Error("CachedHeader after Invalidate = miss, want the stale header")
	}
	if _, err := s.Header(t.Context(), HeaderQuery{}); err != nil {
		t.Fatal(err)
	}
	api.wantCalls(t, "header", "header")
}

// Each read stays fresh for its own TTL: the config's by default, or the
// one given.
func TestTTLs(t *testing.T) {
	d := config.Default().Cache.TTL
	tests := []struct {
		name string
		ttls TTLs
		want time.Duration
		read func(context.Context, *Service) error
	}{
		{"header", TTLs{}, d.Profile, func(ctx context.Context, s *Service) error { _, err := s.Header(ctx, HeaderQuery{}); return err }},
		{"header with a ttl", TTLs{Header: 2 * time.Hour}, 2 * time.Hour, func(ctx context.Context, s *Service) error { _, err := s.Header(ctx, HeaderQuery{}); return err }},
		{"work", TTLs{}, d.WaitingOnYou, func(ctx context.Context, s *Service) error { _, err := s.Work(ctx, WorkQuery{}); return err }},
		{"work with a ttl", TTLs{Work: time.Minute}, time.Minute, func(ctx context.Context, s *Service) error { _, err := s.Work(ctx, WorkQuery{}); return err }},
		{"contributions", TTLs{}, d.Contributions, func(ctx context.Context, s *Service) error {
			_, err := s.Contributions(ctx, ContributionsQuery{})
			return err
		}},
		{"repos", TTLs{}, d.DashboardRepos, func(ctx context.Context, s *Service) error {
			_, err := s.Repos(ctx, ReposQuery{Viewer: true})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				api := &fakeAPI{t: t,
					header:        func() (core.Header, error) { return octocat, nil },
					work:          func(int) (core.Work, error) { return core.Work{}, nil },
					contributions: func() (core.Contributions, error) { return core.Contributions{}, nil },
					ownRepos:      func(int, string) (core.Page[core.Repo], error) { return core.Page[core.Repo]{}, nil },
				}
				s := New(api, WithTTLs(tt.ttls))
				fresh := func(s *Service) bool {
					return s.FreshHeader() || s.FreshWork(WorkQuery{}) || s.FreshContributions() || s.FreshRepos(ReposQuery{Viewer: true})
				}
				if fresh(s) {
					t.Error("fresh before a read = true")
				}
				if err := tt.read(t.Context(), s); err != nil {
					t.Fatal(err)
				}
				time.Sleep(tt.want - time.Second)
				if err := tt.read(t.Context(), s); err != nil {
					t.Fatal(err)
				}
				if n := len(api.Calls()); n != 1 {
					t.Errorf("%d calls within the TTL, want 1", n)
				}
				if !fresh(s) {
					t.Error("fresh within the TTL = false")
				}
				time.Sleep(time.Second)
				if fresh(s) {
					t.Error("fresh once the TTL passed = true")
				}
				if err := tt.read(t.Context(), s); err != nil {
					t.Fatal(err)
				}
				if n := len(api.Calls()); n != 2 {
					t.Errorf("%d calls once the TTL passed, want 2", n)
				}
			})
		})
	}
}

func TestWorkPageSize(t *testing.T) {
	api := &fakeAPI{t: t, work: func(first int) (core.Work, error) {
		return core.Work{Authored: core.WorkList{Count: first}}, nil
	}}
	s := New(api)
	for _, q := range []WorkQuery{{}, {PageSize: 10}, {PageSize: 5}, {PageSize: 500}} {
		if _, err := s.Work(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	api.wantCalls(t, "work 10", "work 5", "work 100")
	if w, ok := s.CachedWork(WorkQuery{PageSize: 1000}); !ok || w.Authored.Count != 100 {
		t.Errorf("CachedWork(1000) = %+v, %v; want the page of 100", w, ok)
	}
	if _, ok := s.CachedWork(WorkQuery{PageSize: 20}); ok {
		t.Error("CachedWork(20) = hit, want a miss")
	}
}

func TestReposByOwner(t *testing.T) {
	api := &fakeAPI{t: t,
		ownRepos: func(int, string) (core.Page[core.Repo], error) {
			return core.Page[core.Repo]{Items: repos("octocat", 0, 1)}, nil
		},
		orgRepos: pagedOrg(150),
	}
	s := New(api)
	// The viewer's own repositories ignore Owner.
	if p, err := s.Repos(t.Context(), ReposQuery{Viewer: true, Owner: "ignored"}); err != nil || len(p.Items) != 1 {
		t.Fatalf("own Repos = %+v, %v", p, err)
	}
	if _, ok := s.CachedRepos(ReposQuery{Viewer: true}); !ok {
		t.Error("CachedRepos(viewer) = miss, want the page")
	}
	p, err := s.Repos(t.Context(), ReposQuery{Owner: "Charm"})
	if err != nil || len(p.Items) != 100 || p.Next != "100" {
		t.Fatalf("org Repos = %d repos next %q, %v; want 100 and a next page", len(p.Items), p.Next, err)
	}
	if p, err = s.Repos(t.Context(), ReposQuery{Owner: "charm", Cursor: p.Next}); err != nil || len(p.Items) != 50 || !p.Last() {
		t.Fatalf("second org page = %d repos next %q, %v; want the last 50", len(p.Items), p.Next, err)
	}
	// Logins match without regard to case, so this is the first page again.
	if _, ok := s.CachedRepos(ReposQuery{Owner: "CHARM"}); !ok {
		t.Error("CachedRepos(CHARM) = miss, want the page of charm")
	}
	if _, ok := s.CachedRepos(ReposQuery{Owner: "charm", PageSize: 30}); ok {
		t.Error("CachedRepos with another page size = hit, want a miss")
	}
	api.wantCalls(t, "own 100 ", "org Charm 100 ", "org charm 100 100")
}

func TestAllRepos(t *testing.T) {
	api := &fakeAPI{t: t, orgRepos: pagedOrg(250)}
	s := New(api)
	q := ReposQuery{Owner: "charm"}
	if _, ok := s.CachedAllRepos(q, 0); ok {
		t.Error("CachedAllRepos before a read = hit, want a miss")
	}
	if _, err := s.Repos(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	if p, ok := s.CachedAllRepos(q, 0); !ok || len(p.Items) != 100 || p.Next != "100" {
		t.Errorf("CachedAllRepos after one page = %d repos next %q, %v; want 100 up to 100", len(p.Items), p.Next, ok)
	}

	p, err := s.AllRepos(t.Context(), q, 0)
	if err != nil || len(p.Items) != 250 || !p.Last() {
		t.Fatalf("AllRepos = %d repos next %q, %v; want all 250", len(p.Items), p.Next, err)
	}
	if p.Items[249].Ref.Name != "repo249" {
		t.Errorf("last repo = %s, want charm/repo249", p.Items[249].Ref)
	}
	// The first page was cached, so AllRepos fetched only the rest.
	api.wantCalls(t, "org charm 100 ", "org charm 100 100", "org charm 100 200")
	if p, ok := s.CachedAllRepos(q, 0); !ok || len(p.Items) != 250 {
		t.Errorf("CachedAllRepos = %d repos, %v; want all 250", len(p.Items), ok)
	}
}

func TestAllReposLimit(t *testing.T) {
	api := &fakeAPI{t: t, orgRepos: pagedOrg(5000)}
	s := New(api)
	p, err := s.AllRepos(t.Context(), ReposQuery{Owner: "big"}, 150)
	if err != nil || len(p.Items) != 200 || p.Next != "200" {
		t.Errorf("AllRepos(150) = %d repos next %q, %v; want two whole pages", len(p.Items), p.Next, err)
	}
	if p, err = s.AllRepos(t.Context(), ReposQuery{Owner: "big"}, 1e6); err != nil || len(p.Items) != MaxOwnerRepos || p.Next == "" {
		t.Errorf("AllRepos(1e6) = %d repos next %q, %v; want MaxOwnerRepos and more to come", len(p.Items), p.Next, err)
	}
	if n := len(api.Calls()); n != MaxOwnerRepos/DefaultReposSize {
		t.Errorf("%d calls, want %d", n, MaxOwnerRepos/DefaultReposSize)
	}
}

func TestAllReposError(t *testing.T) {
	boom := errors.New("boom")
	next := pagedOrg(300)
	api := &fakeAPI{t: t, orgRepos: func(login string, first int, after string) (core.Page[core.Repo], error) {
		if after == "100" {
			return core.Page[core.Repo]{}, boom
		}
		return next(login, first, after)
	}}
	s := New(api)
	if _, err := s.AllRepos(t.Context(), ReposQuery{Owner: "charm"}, 0); !errors.Is(err, boom) {
		t.Errorf("AllRepos error = %v, want boom", err)
	}
	// The pages before the failure stay cached.
	if p, ok := s.CachedAllRepos(ReposQuery{Owner: "charm"}, 0); !ok || len(p.Items) != 100 {
		t.Errorf("CachedAllRepos = %d repos, %v; want the first page", len(p.Items), ok)
	}
}

func TestReadErrors(t *testing.T) {
	missing := errors.Join(errors.New("no such organization"), core.ErrNotFound)
	api := &fakeAPI{t: t,
		header:        func() (core.Header, error) { return core.Header{}, core.ErrUnauthorized },
		work:          func(int) (core.Work, error) { return core.Work{}, core.ErrUnauthorized },
		contributions: func() (core.Contributions, error) { return core.Contributions{}, core.ErrUnauthorized },
		orgRepos:      func(string, int, string) (core.Page[core.Repo], error) { return core.Page[core.Repo]{}, missing },
	}
	s := New(api)
	if _, err := s.Header(t.Context(), HeaderQuery{}); !errors.Is(err, core.ErrUnauthorized) {
		t.Errorf("Header error = %v, want ErrUnauthorized", err)
	}
	if _, err := s.Work(t.Context(), WorkQuery{}); !errors.Is(err, core.ErrUnauthorized) {
		t.Errorf("Work error = %v, want ErrUnauthorized", err)
	}
	if _, err := s.Contributions(t.Context(), ContributionsQuery{}); !errors.Is(err, core.ErrUnauthorized) {
		t.Errorf("Contributions error = %v, want ErrUnauthorized", err)
	}
	if _, err := s.Repos(t.Context(), ReposQuery{Owner: "nope"}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Repos error = %v, want ErrNotFound", err)
	}
	// Errors aren't cached.
	if _, ok := s.CachedHeader(); ok {
		t.Error("CachedHeader after an error = hit, want a miss")
	}
}
