package pulls

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

// etag is the ETag of the probe while the pull requests of v are as they
// are. Like GitHub's, it moves with their update time, and not with their
// checks.
func (v *versioned) etag() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return fmt.Sprintf("%q", v.updated.Format(time.RFC3339Nano))
}

// probing is v.api with the probe that Poll and List send, answering as
// GitHub does.
func (v *versioned) probing() *fakeAPI {
	api := v.api()
	api.probe = func(_ context.Context, _ core.RepoRef, cond github.Conditional) (github.Response, error) {
		etag := v.etag()
		if cond.ETag == etag {
			return github.Response{StatusCode: 304, NotModified: true, ETag: etag}, nil
		}
		return github.Response{StatusCode: 200, ETag: etag}, nil
	}
	return api
}

// probedSession polls the repository, as the app does once it shows it,
// then lists its pull requests and reads #1, so that the page kept in
// store carries the probe's ETag.
func probedSession(t *testing.T, v *versioned, store *disk.Store) {
	t.Helper()
	s := New(v.probing(), WithStore(store))
	if _, err := s.Poll(repo)(t.Context()); err != nil {
		t.Fatal(err)
	}
	list(t, s, openList)
	readDetail(t, s)
}

// revisit lists the pull requests in a session that starts an hour after
// what store keeps, as the tui does: the kept page at once, stale, then the
// page again.
func revisit(t *testing.T, api *fakeAPI, store *disk.Store) (*Service, core.Page[core.PullRequest], error) {
	t.Helper()
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	if p, err := s.List(t.Context(), openList); err != nil || !p.Stale {
		t.Fatalf("first List = %+v, %v; want the kept page, stale", p, err)
	}
	p, err := s.List(t.Context(), openList.again())
	return s, p, err
}

func wantReads(t *testing.T, api *fakeAPI, probe, list int) {
	t.Helper()
	if n := api.count("probe"); n != probe {
		t.Errorf("probe called %d times, want %d", n, probe)
	}
	if n := api.count("list"); n != list {
		t.Errorf("list called %d times, want %d", n, list)
	}
}

func TestKeptListConfirmedByProbe(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	probedSession(t, v, store)

	before := time.Now()
	api := v.probing()
	s, p, err := revisit(t, api, store)
	if err != nil || p.Stale || len(p.Items) != 1 {
		t.Fatalf("List = %+v, %v; want the kept page, confirmed", p, err)
	}
	// One free 304 instead of the GraphQL list.
	wantReads(t, api, 1, 0)
	if !s.FreshList(openList) {
		t.Error("FreshList after the probe = false, want the page fresh")
	}
	// The page vouches for the detail and comments it lists.
	readDetail(t, s)
	wantCalls(t, api, 0, 0)

	// The kept page is marked fetched now, so the next session starts
	// fresh.
	e, ok := New(v.api(), WithStore(store)).keptLists.Load(openList.key(30))
	if !ok || e.FetchedAt.Before(before) || e.ETag != v.etag() {
		t.Errorf("kept page fetched at %v with ETag %s, %v; want at least %v with %s", e.FetchedAt, e.ETag, ok, before, v.etag())
	}
}

func TestKeptListReadWhenProbeChanged(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	probedSession(t, v, store)
	v.set(epoch.Add(time.Minute), core.ChecksSuccess)

	api := v.probing()
	_, p, err := revisit(t, api, store)
	if err != nil || p.Stale || !p.Items[0].UpdatedAt.Equal(epoch.Add(time.Minute)) {
		t.Fatalf("List = %+v, %v; want the page read again", p, err)
	}
	wantReads(t, api, 1, 1)
	// The page read carries the probe's new ETag, which predates it.
	e, ok := New(v.api(), WithStore(store)).keptLists.Load(openList.key(30))
	if !ok || e.ETag != v.etag() {
		t.Errorf("kept page has ETag %s, %v; want %s", e.ETag, ok, v.etag())
	}

	// So the next session confirms it for free.
	api = v.probing()
	if _, _, err := revisit(t, api, store); err != nil {
		t.Fatal(err)
	}
	wantReads(t, api, 1, 0)
}

func TestKeptListWithPendingChecksIsRead(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksPending}
	store := openStore(t)
	probedSession(t, v, store)
	// The checks finish, which doesn't move the update time, nor the
	// probe's ETag.
	v.set(epoch, core.ChecksSuccess)

	api := v.probing()
	_, p, err := revisit(t, api, store)
	if err != nil || p.Items[0].Checks != core.ChecksSuccess {
		t.Fatalf("List = %+v, %v; want the page read again, with the checks done", p, err)
	}
	wantReads(t, api, 1, 1)
}

func TestKeptListWithoutETagIsRead(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	// Listed before any poll, so no probe vouches for the page.
	firstSession(t, v, store)

	// It is read again, after a probe that vouches for it from then on.
	api := v.probing()
	if _, _, err := revisit(t, api, store); err != nil {
		t.Fatal(err)
	}
	wantReads(t, api, 1, 1)
	api = v.probing()
	if _, _, err := revisit(t, api, store); err != nil {
		t.Fatal(err)
	}
	wantReads(t, api, 1, 0)
}

func TestNewListNotProbed(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	api := v.probing()
	s := New(api)
	// A page read for the first time doesn't wait for a probe.
	list(t, s, openList)
	wantReads(t, api, 0, 1)
	// Once Poll probed, the ETag it brought vouches for a page read after.
	if _, err := s.Poll(repo)(t.Context()); err != nil {
		t.Fatal(err)
	}
	q := ListQuery{Repo: repo, State: core.StateClosed}
	list(t, s, q)
	if e, _ := s.lists.Get(q.key(30)); e.ETag != v.etag() {
		t.Errorf("page read after Poll has ETag %q, want %q", e.ETag, v.etag())
	}
}

func TestSearchListNotProbed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := &versioned{updated: epoch, checks: core.ChecksSuccess}
		api := v.probing()
		api.search = func(context.Context, string, string, int) (core.Page[core.PullRequest], error) {
			return core.Page[core.PullRequest]{Items: []core.PullRequest{v.pull()}}, nil
		}
		s := New(api, WithTTL(time.Minute))
		if _, err := s.Poll(repo)(t.Context()); err != nil {
			t.Fatal(err)
		}
		q := ListQuery{Repo: repo, State: core.StateOpen, Filter: "author:@me"}
		list(t, s, q)
		time.Sleep(2 * time.Minute)
		list(t, s, q)
		// A search may lag behind the probe, so it is read again.
		if n, m := api.count("search"), api.count("probe"); n != 2 || m != 1 {
			t.Errorf("search called %d times and probe %d, want 2 and only Poll's", n, m)
		}
	})
}

func TestKeptListProbeFails(t *testing.T) {
	tests := []struct {
		name string
		err  error
		// offline is whether the kept page is served offline, and read
		// whether the list is read without the probe.
		offline, read bool
	}{
		{name: "unreachable", err: errDial, offline: true},
		{name: "server error", err: &github.Error{StatusCode: 502}, offline: true},
		{name: "not found", err: &github.Error{StatusCode: 404}},
		{name: "rate limited", err: &core.RateLimitError{Reset: epoch}, read: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &versioned{updated: epoch, checks: core.ChecksSuccess}
			store := openStore(t)
			probedSession(t, v, store)

			api := v.probing()
			api.probe = func(context.Context, core.RepoRef, github.Conditional) (github.Response, error) {
				return github.Response{}, tt.err
			}
			_, p, err := revisit(t, api, store)
			switch {
			case tt.offline:
				if err != nil || !p.Offline {
					t.Errorf("List = %+v, %v; want the kept page, offline", p, err)
				}
				wantReads(t, api, 1, 0)
			case tt.read:
				if err != nil || p.Offline || p.Stale {
					t.Errorf("List = %+v, %v; want the page read", p, err)
				}
				wantReads(t, api, 1, 1)
			default:
				if err == nil {
					t.Errorf("List = %+v; want the refusal", p)
				}
				if _, ok := New(v.api(), WithStore(store)).keptLists.Load(openList.key(30)); ok {
					t.Error("the refused page is still kept")
				}
			}
		})
	}
}

// TestListInvalidatedWhileRead checks that a poll that finds a change while
// the list is read, as the first poll of a session does, doesn't cost a
// second read of the list.
func TestListInvalidatedWhileRead(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	probedSession(t, v, store)
	v.set(epoch.Add(time.Minute), core.ChecksSuccess)

	api := v.probing()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	read := api.list
	api.list = func(ctx context.Context, r core.RepoRef, st core.State, cursor string, first int) (core.Page[core.PullRequest], error) {
		once.Do(func() { close(started) })
		<-release
		return read(ctx, r, st, cursor, first)
	}
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	if p, err := s.List(t.Context(), openList); err != nil || !p.Stale {
		t.Fatalf("first List = %+v, %v; want the kept page, stale", p, err)
	}
	done := make(chan error)
	go func() {
		_, err := s.List(t.Context(), openList.again())
		done <- err
	}()
	<-started
	// The session's first poll asks with the ETag the last one kept, and
	// finds the change the list is being read for.
	if res, err := s.Poll(repo)(t.Context()); err != nil || !res.Changed {
		t.Fatalf("Poll = %+v, %v; want a change", res, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if p, ok := s.CachedList(openList); !ok || !p.Items[0].UpdatedAt.Equal(epoch.Add(time.Minute)) {
		t.Errorf("CachedList = %+v, %v; want the page just read", p, ok)
	}

	// The change event reads the list again, which a probe confirms.
	p, err := s.List(t.Context(), openList)
	if err != nil || !p.Items[0].UpdatedAt.Equal(epoch.Add(time.Minute)) {
		t.Fatalf("List after the change = %+v, %v", p, err)
	}
	// The list's probe, Poll's, and the list's again.
	wantReads(t, api, 3, 1)
	if _, st := s.lists.Get(openList.key(30)); st != cache.Fresh {
		t.Errorf("page is %v, want fresh", st)
	}
}

// TestRefreshReadsListAgain checks that a refresh the user asked for reads
// the list in full, as it may be for what the probe can't see, such as
// checks run again.
func TestRefreshReadsListAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := &versioned{updated: epoch, checks: core.ChecksSuccess}
		api := v.probing()
		s := New(api)
		if _, err := s.Poll(repo)(t.Context()); err != nil {
			t.Fatal(err)
		}
		list(t, s, openList)
		// The checks run again, which moves no update time, and the user
		// refreshes at the same instant as the read.
		v.set(epoch, core.ChecksPending)
		s.Invalidate(repo)
		p, err := s.List(t.Context(), openList)
		if err != nil || p.Items[0].Checks != core.ChecksPending {
			t.Fatalf("List after a refresh = %+v, %v; want the checks running", p, err)
		}
		// Poll's and the refresh's probes, which still stamp the page.
		wantReads(t, api, 2, 2)

		// A change a probe found may be confirmed, as it doesn't come
		// from the user, once the page is read after the refresh.
		time.Sleep(time.Second)
		v.set(epoch, core.ChecksSuccess)
		s.invalidate(repo)
		list(t, s, openList)
		// The page has checks running, so it is read.
		wantReads(t, api, 3, 3)
		s.invalidate(repo)
		list(t, s, openList)
		wantReads(t, api, 4, 3)

		// A later refresh reads it again.
		time.Sleep(time.Second)
		s.Invalidate(repo)
		list(t, s, openList)
		wantReads(t, api, 5, 4)
	})
}

func TestChecksLimitConfirmation(t *testing.T) {
	tests := []struct {
		name   string
		checks core.ChecksState
		// after is how long after the full read the page is read again.
		after time.Duration
		reads int
	}{
		{"failing within the window", core.ChecksFailure, failingFor*time.Minute - time.Second, 1},
		{"failing past the window", core.ChecksFailure, failingFor * time.Minute, 2},
		{"pending", core.ChecksPending, 2 * time.Minute, 2},
		{"passed", core.ChecksSuccess, failingFor * time.Minute, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				v := &versioned{updated: epoch, checks: tt.checks}
				api := v.probing()
				s := New(api, WithTTL(time.Minute))
				if _, err := s.Poll(repo)(t.Context()); err != nil {
					t.Fatal(err)
				}
				list(t, s, openList)
				// The checks are run again and pass, which moves no update
				// time, nor the probe's ETag.
				v.set(epoch, core.ChecksSuccess)
				time.Sleep(tt.after)
				p, err := s.List(t.Context(), openList)
				if err != nil {
					t.Fatal(err)
				}
				wantReads(t, api, 2, tt.reads)
				if want := map[int]core.ChecksState{1: tt.checks, 2: core.ChecksSuccess}[tt.reads]; p.Items[0].Checks != want {
					t.Errorf("checks = %q, want %q", p.Items[0].Checks, want)
				}
			})
		})
	}
}

func TestConfirmedOnlyForAWhile(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := &versioned{updated: epoch, checks: core.ChecksSuccess}
		api := v.probing()
		s := New(api, WithTTL(time.Minute))
		if _, err := s.Poll(repo)(t.Context()); err != nil {
			t.Fatal(err)
		}
		list(t, s, openList)
		// Each probe confirms the page for another TTL, until the page
		// was read in full confirmFor ago.
		reads := 0
		for range int(confirmFor/time.Minute) + 1 {
			time.Sleep(time.Minute)
			list(t, s, openList)
			reads = api.count("list")
			if reads > 1 {
				break
			}
		}
		if reads != 2 {
			t.Fatalf("list read %d times, want once more after %v", reads, confirmFor)
		}
		if n := api.count("probe") - 1; n != int(confirmFor/time.Minute) {
			t.Errorf("probed %d times after Poll, want %d", n, int(confirmFor/time.Minute))
		}
	})
}

// TestFailedChangeNotConfirmed checks that a change that failed isn't kept
// on the list, although a probe confirmed the page while it was sent.
func TestFailedChangeNotConfirmed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := &versioned{updated: epoch, checks: core.ChecksSuccess}
		api := v.probing()
		release := make(chan struct{})
		api.mutate = func(context.Context, string, string, core.MergeMethod) (core.PullRequest, error) {
			<-release
			return core.PullRequest{}, core.ErrConflict
		}
		s := New(api, WithTTL(time.Minute))
		if _, err := s.Poll(repo)(t.Context()); err != nil {
			t.Fatal(err)
		}
		list(t, s, openList)
		time.Sleep(2 * time.Minute)

		op := s.Close(repo, 1)
		done := make(chan error)
		go func() { done <- op.Do(t.Context()) }()
		synctest.Wait()
		// The stale page is confirmed while the close is sent, and shows
		// it.
		if p, err := s.List(t.Context(), openList); err != nil || p.Items[0].State != core.StateClosed {
			t.Fatalf("List while closing = %+v, %v; want it closed", p, err)
		}
		close(release)
		if err := <-done; err == nil {
			t.Fatal("Do succeeded, want the failure")
		}
		p, err := s.List(t.Context(), openList)
		if err != nil || p.Items[0].State != core.StateOpen {
			t.Errorf("List after the failed close = %+v, %v; want it open", p, err)
		}
		// The page is read again in full, not confirmed.
		wantReads(t, api, 3, 2)
	})
}

// keptListCheck checks the kept open list of store with the revalidator's
// entry, in a session an hour later, whose clock reads at, or now if at is
// zero.
func keptListCheck(t *testing.T, v *versioned, store *disk.Store, at time.Time) (revalidate.Result, *fakeAPI) {
	t.Helper()
	id := kindList + ":" + openList.key(30)
	api := v.probing()
	s := New(api, WithStore(cachetest.Aged(store, time.Hour)))
	if !at.IsZero() {
		s.now = func() time.Time { return at }
	}
	for _, e := range s.Kept() {
		if e.ID == id {
			return e.Check(t.Context()), api
		}
	}
	t.Fatalf("Kept lacks %s", id)
	return revalidate.Result{}, nil
}

// TestKeptListRevalidated checks that the revalidator lists a kept list
// page, and confirms it with the free probe while no pull request changed,
// but once one did, reports the change to the views without reading it.
func TestKeptListRevalidated(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	probedSession(t, v, store)

	res, api := keptListCheck(t, v, store, time.Time{})
	if res.Status != revalidate.NotModified {
		t.Errorf("check = %+v, want not modified", res)
	}
	wantReads(t, api, 1, 0)

	v.set(epoch.Add(time.Minute), core.ChecksSuccess)
	res, api = keptListCheck(t, v, store, time.Time{})
	// The probe that found the change was a request, so it counts as one,
	// and the views showing the page read it again.
	if want := (revalidate.Result{Status: revalidate.Changed, Sync: SyncKey(repo)}); res != want {
		t.Errorf("check after a change = %+v, want %+v", res, want)
	}
	wantReads(t, api, 1, 0)
}

// TestKeptListUnconfirmableSkipped checks that the revalidator spends
// nothing on a kept page the probe can't vouch for: one read in full too
// long ago, or that shows checks running.
func TestKeptListUnconfirmableSkipped(t *testing.T) {
	tests := []struct {
		name   string
		checks core.ChecksState
		// later is how long after the page was read the check runs.
		later time.Duration
	}{
		{"old", core.ChecksSuccess, confirmFor + time.Minute},
		{"checks running", core.ChecksPending, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &versioned{updated: epoch, checks: tt.checks}
			store := openStore(t)
			probedSession(t, v, store)
			var at time.Time
			if tt.later > 0 {
				// The page's ReadAt is this session's clock, which Aged
				// doesn't move.
				at = time.Now().Add(tt.later)
			}
			res, api := keptListCheck(t, v, store, at)
			if res.Status != revalidate.Skipped {
				t.Errorf("check = %+v, want skipped", res)
			}
			wantReads(t, api, 0, 0)
		})
	}
}

func TestParseListKey(t *testing.T) {
	for _, size := range []int{30, 50} {
		for _, q := range []ListQuery{openList, {Repo: repo}, {Repo: repo, State: core.StateClosed, Cursor: "Y3Vyc29yOjI=", PageSize: 50}} {
			got, ok := parseListKey(q.key(size), size)
			if !ok || got.key(size) != q.key(size) {
				t.Errorf("parseListKey(%q, %d) = %+v, %v; want %+v", q.key(size), size, got, ok, q)
			}
		}
	}
	for _, key := range []string{"pulls:eggzec/gh-tui", "pull:eggzec/gh-tui?state=open", (ListQuery{Repo: repo, Filter: "label:bug"}).key(30)} {
		if _, ok := parseListKey(key, 30); ok {
			t.Errorf("parseListKey(%q) ok, want refused", key)
		}
	}
}
