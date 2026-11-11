package releases

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

// listAPI serves two releases, the second the latest, and counts the reads.
type listAPI struct {
	fakeAPI
	lists, latests int
	// listErr and latestErr fail the reads. changed serves a new page with a
	// new ETag, whatever the request has. paged gives the first page a Next.
	listErr, latestErr error
	changed, paged     bool
	cursors            []string
	perPages           []int
}

const page2 = "https://api.github.com/repositories/1/releases?page=2"

func (f *listAPI) ListReleases(_ context.Context, _ core.RepoRef, cursor string, perPage int, cond github.Conditional) (core.Page[core.Release], github.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists++
	f.cursors = append(f.cursors, cursor)
	f.perPages = append(f.perPages, perPage)
	if f.listErr != nil {
		return core.Page[core.Release]{}, github.Response{}, f.listErr
	}
	if f.changed {
		return core.Page[core.Release]{Items: []core.Release{{ID: 3, Tag: "v3"}}}, github.Response{ETag: `"l2"`}, nil
	}
	if cond.ETag == `"l1"` {
		return core.Page[core.Release]{}, github.Response{NotModified: true, ETag: `"l1"`}, nil
	}
	if cursor == page2 {
		return core.Page[core.Release]{Items: []core.Release{{ID: 0, Tag: "v0"}}}, github.Response{ETag: `"l1"`}, nil
	}
	p := core.Page[core.Release]{Items: []core.Release{{ID: 2, Tag: "v2"}, {ID: 1, Tag: "v1"}}}
	if f.paged {
		p.Next = page2
	}
	return p, github.Response{ETag: `"l1"`}, nil
}

func (f *listAPI) LatestRelease(context.Context, core.RepoRef) (core.Release, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.latests++
	if f.latestErr != nil {
		return core.Release{}, false, f.latestErr
	}
	return core.Release{ID: 1}, true, nil
}

func (f *listAPI) reads() (lists, latests int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lists, f.latests
}

func (f *listAPI) ReleaseAssetURL(context.Context, core.RepoRef, int64) (string, error) {
	return "https://example.com/signed", nil
}

func TestListMarksLatest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &listAPI{}
		s := New(api, WithTTL(time.Minute))
		if _, ok := s.CachedList(ListQuery{Repo: repo}); ok {
			t.Fatal("a page is cached before it was read")
		}
		p, err := s.List(t.Context(), ListQuery{Repo: repo})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(p.Items) != 2 || p.Items[0].Latest || !p.Items[1].Latest {
			t.Errorf("items = %+v, want only v1 marked latest", p.Items)
		}
		if c, ok := s.CachedList(ListQuery{Repo: repo}); !ok || !c.Items[1].Latest {
			t.Errorf("CachedList = %+v, %v; want the marked page", c, ok)
		}

		// A fresh page is a hit: neither list nor latest is read again.
		if _, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil {
			t.Fatalf("List: %v", err)
		}
		if api.lists != 1 || api.latests != 1 {
			t.Errorf("reads = %d lists, %d latests, want 1 and 1", api.lists, api.latests)
		}

		// Past the TTL the page is revalidated with its ETag.
		time.Sleep(time.Minute)
		if p, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil || len(p.Items) != 2 {
			t.Errorf("List after a 304 = %+v, %v; want the cached page", p, err)
		}
		if api.lists != 2 {
			t.Errorf("lists = %d, want 2", api.lists)
		}
	})
}

func TestInvalidateMakesTheNextListAsk(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &listAPI{}
		s := New(api, WithTTL(time.Minute))
		if _, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil {
			t.Fatalf("List: %v", err)
		}
		s.Invalidate(core.RepoRef{Owner: "other", Name: "repo"})
		if _, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil || api.lists != 1 {
			t.Fatalf("after another repo's Invalidate: lists = %d, err = %v; want 1", api.lists, err)
		}
		s.Invalidate(repo)
		if _, ok := s.CachedList(ListQuery{Repo: repo}); !ok {
			t.Error("CachedList lost the page, which Invalidate only marks stale")
		}
		p, err := s.List(t.Context(), ListQuery{Repo: repo})
		if err != nil || len(p.Items) != 2 {
			t.Fatalf("List = %+v, %v; want the page", p, err)
		}
		if api.lists != 2 {
			t.Errorf("lists = %d, want 2: Invalidate must make the next List ask", api.lists)
		}
	})
}

// keptStore returns a store that holds a page an earlier session read, aged
// past the TTL. It must run inside a synctest bubble.
func keptStore(t *testing.T, ttl time.Duration, q ListQuery) *disk.Store {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(&listAPI{}, WithStore(store), WithTTL(ttl)).List(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	// A kept page fetched within the TTL is fresh, so let it age.
	time.Sleep(2 * ttl)
	return store
}

func TestListKeptIsStaleUntilAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := keptStore(t, time.Minute, ListQuery{Repo: repo})

		api := &listAPI{}
		s := New(api, WithStore(store), WithTTL(time.Minute))
		if _, ok := s.CachedList(ListQuery{Repo: repo}); ok {
			t.Error("CachedList found a kept page, which only a List warms")
		}
		p, err := s.List(t.Context(), ListQuery{Repo: repo})
		if err != nil || !p.Stale {
			t.Fatalf("List = %+v, %v; want the kept page, stale", p, err)
		}
		if again, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil || !again.Stale {
			t.Errorf("second List = %+v, %v; want stale until Again", again, err)
		}
		if lists, latests := api.reads(); lists != 0 || latests != 0 {
			t.Errorf("kept reads = %d lists, %d latests, want none", lists, latests)
		}
		p, err = s.List(t.Context(), ListQuery{Repo: repo, Again: true})
		if err != nil || p.Stale {
			t.Errorf("List with Again = %+v, %v; want a revalidated page", p, err)
		}
	})
}

func TestListLatestFailureIsNotRetried(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &listAPI{latestErr: core.ErrOffline}
		s := New(api, WithTTL(time.Minute))
		for range 3 {
			p, err := s.List(t.Context(), ListQuery{Repo: repo})
			if err != nil || len(p.Items) != 2 || p.Items[0].Latest || p.Items[1].Latest {
				t.Fatalf("List = %+v, %v; want the page with no mark", p, err)
			}
		}
		if lists, latests := api.reads(); lists != 1 || latests != 1 {
			t.Errorf("reads = %d lists, %d latests, want 1 and 1", lists, latests)
		}
	})
}

func TestListPagination(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &listAPI{paged: true}
		s := New(api, WithTTL(time.Minute), WithPageSize(7))
		first, err := s.List(t.Context(), ListQuery{Repo: repo})
		if err != nil || first.Next != page2 || len(first.Items) != 2 {
			t.Fatalf("first page = %+v, %v", first, err)
		}
		second, err := s.List(t.Context(), ListQuery{Repo: repo, Cursor: first.Next})
		if err != nil || second.Next != "" || len(second.Items) != 1 || second.Items[0].Tag != "v0" {
			t.Fatalf("second page = %+v, %v", second, err)
		}
		if got := api.cursors; len(got) != 2 || got[0] != "" || got[1] != page2 {
			t.Errorf("cursors = %q, want the empty one and %q", got, page2)
		}
		if got := api.perPages; got[0] != 7 || got[1] != 7 {
			t.Errorf("per page = %v, want the service's size, 7", got)
		}
		// Each page has a key of its own, and a hit reads nothing.
		for cursor, tag := range map[string]string{"": "v2", page2: "v0"} {
			if c, ok := s.CachedList(ListQuery{Repo: repo, Cursor: cursor}); !ok || c.Items[0].Tag != tag {
				t.Errorf("CachedList(%q) = %+v, %v; want a page starting at %s", cursor, c, ok, tag)
			}
		}
		if _, err := s.List(t.Context(), ListQuery{Repo: repo, Cursor: page2}); err != nil {
			t.Fatal(err)
		}
		if lists, _ := api.reads(); lists != 2 {
			t.Errorf("lists = %d, want 2", lists)
		}

		// A size is part of the key and is clamped to what GitHub allows.
		if _, err := s.List(t.Context(), ListQuery{Repo: repo, PageSize: 500}); err != nil {
			t.Fatal(err)
		}
		if got := api.perPages[len(api.perPages)-1]; got != 100 {
			t.Errorf("per page = %d, want 100", got)
		}
		if _, ok := s.CachedList(ListQuery{Repo: repo, PageSize: 100}); !ok {
			t.Error("the clamped size has no page cached")
		}
	})
}

func TestListFallback(t *testing.T) {
	for _, tt := range []struct {
		name         string
		err          error
		wantKept     bool
		offline, lim bool
	}{
		{"offline", core.ErrOffline, true, true, false},
		{"rate limited", &core.RateLimitError{}, true, false, true},
		{"not found", core.ErrNotFound, false, false, false},
		{"forbidden", core.ErrForbidden, false, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				api := &listAPI{}
				s := New(api, WithTTL(time.Minute))
				if _, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil {
					t.Fatal(err)
				}
				time.Sleep(2 * time.Minute)
				api.listErr = tt.err
				p, err := s.List(t.Context(), ListQuery{Repo: repo})
				if !tt.wantKept {
					if err == nil {
						t.Fatalf("List = %+v, want the error, not the kept page", p)
					}
					return
				}
				if err != nil || len(p.Items) != 2 || p.Offline != tt.offline || p.Limited != tt.lim {
					t.Errorf("List = %+v, %v; want the page, offline %v, limited %v", p, err, tt.offline, tt.lim)
				}
			})
		})
	}
}

func TestCachedListDoesNoIO(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := keptStore(t, time.Minute, ListQuery{Repo: repo})
		api := &listAPI{}
		s := New(api, WithStore(store), WithTTL(time.Minute))
		if _, ok := s.CachedList(ListQuery{Repo: repo}); ok {
			t.Error("CachedList found a page nothing read")
		}
		if lists, latests := api.reads(); lists != 0 || latests != 0 {
			t.Errorf("reads = %d lists, %d latests, want none", lists, latests)
		}
	})
}

func TestKeptRevalidatesPages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// A cursor is an address, so its colons are part of the key.
		q := ListQuery{Repo: repo, Cursor: page2}
		store := keptStore(t, time.Minute, q)
		api := &listAPI{}
		s := New(api, WithStore(store), WithTTL(time.Minute))

		entries := s.Kept()
		if len(entries) != 1 {
			t.Fatalf("Kept = %d entries, want the one page", len(entries))
		}
		if entries[0].Repo != repo {
			t.Errorf("entry repo = %v, want %v", entries[0].Repo, repo)
		}
		if res := entries[0].Check(t.Context()); res.Status != revalidate.NotModified {
			t.Fatalf("Check = %+v, want a 304", res)
		}
		if got := api.cursors[len(api.cursors)-1]; got != page2 {
			t.Errorf("checked cursor = %q, want %q", got, page2)
		}

		api.changed = true
		// The 304 marked the entry fetched now; a changed page comes with a
		// full read, whatever its validators.
		res := entries[0].Check(t.Context())
		if res.Status != revalidate.Changed || res.Sync != SyncKey(repo) {
			t.Errorf("Check = %+v, want Changed under %q", res, SyncKey(repo))
		}
	})
}

func TestAssetURL(t *testing.T) {
	got, err := New(&listAPI{}).AssetURL(t.Context(), repo, 5)
	if err != nil || got != "https://example.com/signed" {
		t.Errorf("AssetURL = %q, %v", got, err)
	}
}
