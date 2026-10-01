package pulls

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var (
	epoch    = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	openList = ListQuery{Repo: repo, State: core.StateOpen}
	errDial  = fmt.Errorf("%w: %w", core.ErrOffline, &url.Error{Op: "Post", URL: "https://api.github.com/graphql", Err: errors.New("connection refused")})
)

// failing makes every read of api fail with the error that err returns,
// while it returns one.
func failing(api *fakeAPI, err func() error) *fakeAPI {
	list, get, comments := api.list, api.get, api.comments
	api.list = func(ctx context.Context, r core.RepoRef, st core.State, cursor string, first int) (core.Page[core.PullRequest], error) {
		if err := err(); err != nil {
			return core.Page[core.PullRequest]{}, err
		}
		return list(ctx, r, st, cursor, first)
	}
	api.get = func(ctx context.Context, r core.RepoRef, n int) (core.PullRequestDetail, error) {
		if err := err(); err != nil {
			return core.PullRequestDetail{}, err
		}
		return get(ctx, r, n)
	}
	api.comments = func(ctx context.Context, r core.RepoRef, n int, cursor string, first int) (core.Page[core.Comment], error) {
		if err := err(); err != nil {
			return core.Page[core.Comment]{}, err
		}
		return comments(ctx, r, n, cursor, first)
	}
	return api
}

func openStore(t *testing.T) *disk.Store {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// firstSession lists the pull requests of v and reads #1 into store.
func firstSession(t *testing.T, v *versioned, store *disk.Store) {
	t.Helper()
	s := New(v.api(), WithStore(cachetest.Aged(store, time.Hour)))
	list(t, s, openList)
	readDetail(t, s)
}

func TestKeptListIsServedStaleThenRefetched(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	firstSession(t, v, store)

	api := v.api()
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	p, err := s.List(t.Context(), openList)
	if err != nil || !p.Stale || len(p.Items) != 1 {
		t.Fatalf("List in a new session = %+v, %v; want the kept page, stale", p, err)
	}
	if n := api.count("list"); n != 0 {
		t.Errorf("list called %d times for the kept page, want 0", n)
	}
	// GraphQL has no validators, so the page is fetched again in full.
	p, err = s.List(t.Context(), openList.again())
	if err != nil || p.Stale {
		t.Fatalf("second List = %+v, %v; want the page fetched", p, err)
	}
	if n := api.count("list"); n != 1 {
		t.Errorf("list called %d times, want 1", n)
	}
}

func TestFreshList(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	s := New(v.api(), WithStore(store))
	if s.FreshList(openList) {
		t.Fatal("FreshList before any read = true, want false")
	}
	list(t, s, openList)
	if !s.FreshList(openList) {
		t.Error("FreshList after List = false, want true")
	}
	if s.FreshList(ListQuery{Repo: repo, State: core.StateClosed}) {
		t.Error("FreshList of another state = true, want false")
	}
	s.Invalidate(repo)
	if s.FreshList(openList) {
		t.Error("FreshList after Invalidate = true, want false")
	}

	// A page an earlier session kept within the TTL is fresh.
	if next := New(v.api(), WithStore(store)); !next.FreshList(openList) {
		t.Error("FreshList of a page kept within the TTL = false, want true")
	}
}

func TestFreshListOfPageKeptLongAgo(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	firstSession(t, v, store)

	api := v.api()
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	if s.FreshList(openList) {
		t.Fatal("FreshList of a page kept an hour ago = true, want false")
	}
	// A read ahead fetches it rather than serving the kept page.
	p, err := s.List(t.Context(), openList.again())
	if err != nil || p.Stale || len(p.Items) != 1 {
		t.Fatalf("List = %+v, %v; want the page fetched", p, err)
	}
	if n := api.count("list"); n != 1 {
		t.Errorf("list called %d times, want 1", n)
	}
}

func TestKeptDetailIsCurrentAfterRestart(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	firstSession(t, v, store)

	api := v.api()
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	list(t, s, openList)
	relist(t, s, openList)
	readDetail(t, s)
	// The list still shows the version kept, so the detail and comments
	// cost nothing.
	wantCalls(t, api, 0, 0)
	if !current(s, firstComments) {
		t.Error("Current = false, want true once read from the store")
	}
}

func TestKeptDetailRefetchedWhenNewer(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	firstSession(t, v, store)
	v.set(epoch.Add(time.Hour), core.ChecksSuccess)

	api := v.api()
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	list(t, s, openList)
	relist(t, s, openList)
	readDetail(t, s)
	wantCalls(t, api, 1, 1)
	if d, _ := s.CachedGet(repo, 1); !d.UpdatedAt.Equal(epoch.Add(time.Hour)) {
		t.Errorf("detail updated at %v, want GitHub's newer one", d.UpdatedAt)
	}
}

func TestKeptOffline(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		fallback bool
	}{
		{"unreachable", errDial, true},
		{"server error", &github.Error{StatusCode: 502}, true},
		{"rate limited", fmt.Errorf("graphql: %w", &core.RateLimitError{Reset: epoch}), true},
		{"not found", &github.Error{StatusCode: 404}, false},
		{"unauthorized", &github.Error{StatusCode: 401}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &versioned{updated: epoch, checks: core.ChecksSuccess}
			store := openStore(t)
			firstSession(t, v, store)
			var mu sync.Mutex
			fail := tt.err
			failure := func() error {
				mu.Lock()
				defer mu.Unlock()
				return fail
			}

			s := New(failing(v.api(), failure), WithStore(cachetest.Aged(store, time.Hour)))
			list(t, s, openList)
			p, err := s.List(t.Context(), openList.again())
			other := New(failing(v.api(), failure), WithStore(cachetest.Aged(store, time.Hour)))
			d, getErr := other.Get(t.Context(), repo, 1)
			c, commentsErr := other.Comments(t.Context(), firstComments)
			if !tt.fallback {
				if err == nil || getErr == nil || commentsErr == nil {
					t.Errorf("errors = %v, %v, %v; want each read to fail", err, getErr, commentsErr)
				}
				return
			}
			limited := errors.Is(tt.err, core.ErrRateLimited)
			if err != nil || p.Offline == limited || p.Limited != limited || len(p.Items) != 1 {
				t.Errorf("List = %+v, %v; want the kept page, offline or limited", p, err)
			}
			if getErr != nil || d.Number != 1 {
				t.Errorf("Get = %+v, %v; want the kept detail", d, getErr)
			}
			if commentsErr != nil || c.Offline == limited || c.Limited != limited || len(c.Items) != 1 {
				t.Errorf("Comments = %+v, %v; want the kept page, offline or limited", c, commentsErr)
			}
		})
	}
}

func TestKeptRefusalDropsKept(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	firstSession(t, v, store)
	refused := func() error { return &github.Error{StatusCode: 404} }

	s := New(failing(v.api(), refused), WithStore(cachetest.Aged(store, time.Hour)))
	list(t, s, openList)
	if _, err := s.List(t.Context(), openList.again()); err == nil {
		t.Fatal("List succeeded, want the refusal")
	}
	if current(s, firstComments) {
		t.Error("Current = true after a refusal, want the kept page's vouching forgotten")
	}
	if _, err := s.Get(t.Context(), repo, 1); err == nil {
		t.Fatal("Get succeeded, want the refusal")
	}

	api := v.api()
	next := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	if p, _ := next.List(t.Context(), openList); p.Stale {
		t.Error("List after a refusal = stale, want the kept page gone")
	}
	if _, err := next.Get(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	if n := api.count("get"); n != 1 {
		t.Errorf("get called %d times, want 1: the kept detail is gone", n)
	}
}

func TestKeptMutation(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		t.Run(fmt.Sprintf("confirmed=%v", confirm), func(t *testing.T) {
			v := &versioned{updated: epoch, checks: core.ChecksSuccess}
			store := openStore(t)
			api := v.api()
			api.mutate = func(context.Context, string, string, core.MergeMethod) (core.PullRequest, error) {
				if !confirm {
					return core.PullRequest{}, core.ErrConflict
				}
				pr := v.pull()
				pr.State, pr.UpdatedAt = core.StateClosed, epoch.Add(time.Minute)
				return pr, nil
			}
			s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
			list(t, s, openList)
			readDetail(t, s)
			_ = s.Close(repo, 1).Do(t.Context())

			e, ok := New(v.api(), WithStore(cachetest.Aged(store, time.Hour))).keptDetails.Load(detailKey(repo, 1))
			want := map[bool]core.State{false: core.StateOpen, true: core.StateClosed}[confirm]
			if !ok || e.Value.State != want {
				t.Errorf("kept detail = %+v, %v; want state %s", e.Value, ok, want)
			}
			p, _ := New(v.api(), WithStore(cachetest.Aged(store, time.Hour))).List(t.Context(), openList)
			if p.Items[0].State != core.StateOpen {
				t.Errorf("kept list shows %s, want open until the list is read again", p.Items[0].State)
			}
		})
	}
}

// TestKeptOlderSchemaIsMiss checks that what an older version kept, with
// a shape of its own, is read again rather than decoded wrongly.
func TestKeptOlderSchemaIsMiss(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	key := detailKey(repo, 1)
	old := cache.NewShelf[core.PullRequestDetail](store, kindDetail, detailSchema-1)
	if err := old.Save(key, cache.Entry[core.PullRequestDetail]{Value: core.PullRequestDetail{PullRequest: v.pull()}}); err != nil {
		t.Fatal(err)
	}

	api := v.api()
	s := New(api, WithStore(store))
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	if n := api.count("get"); n != 1 {
		t.Errorf("get called %d times, want 1: the older detail is a miss", n)
	}
	if _, ok := s.keptDetails.Load(key); !ok {
		t.Error("the detail fetched again isn't kept in its place")
	}
}
