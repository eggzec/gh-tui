package pulls

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// threadServer serves two pages of review threads, t1 and t2 on the first and
// t3 on the second, and records the cursor and size of each request.
type threadServer struct {
	mu    sync.Mutex
	err   error
	reads []threadRead
}

type threadRead struct {
	cursor string
	first  int
}

func (r *threadServer) serve(_ context.Context, _ core.RepoRef, _ int, cursor string, first int) (core.ThreadPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads = append(r.reads, threadRead{cursor, first})
	if r.err != nil {
		return core.ThreadPage{}, r.err
	}
	thread := func(id string) core.Thread {
		return core.Thread{ID: id, Path: id + ".go", Line: 3, ViewerCanResolve: true, Comments: []core.ThreadComment{{ID: id + "c", Body: "hm"}}, TotalComments: 1}
	}
	if cursor == "p2" {
		return core.ThreadPage{Threads: []core.Thread{thread("t3")}, Total: 3}, nil
	}
	return core.ThreadPage{Threads: []core.Thread{thread("t1"), thread("t2")}, Total: 3, Next: "p2"}, nil
}

func (r *threadServer) api() *fakeAPI { return &fakeAPI{threads: r.serve} }

func (r *threadServer) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

func (r *threadServer) requests() []threadRead {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]threadRead(nil), r.reads...)
}

var threadsQ = ThreadsQuery{Repo: repo, Number: 1, Head: "abc123"}

func readThreads(t *testing.T, s *Service, q ThreadsQuery) core.ThreadPage {
	t.Helper()
	p, err := s.Threads(t.Context(), q)
	if err != nil {
		t.Fatalf("Threads: %v", err)
	}
	return p
}

func TestThreadsMissThenHit(t *testing.T) {
	r := &threadServer{}
	s := New(r.api())
	if _, ok := s.CachedThreads(threadsQ); ok {
		t.Fatal("CachedThreads before any read = true, want false")
	}
	p := readThreads(t, s, threadsQ)
	if len(p.Threads) != 2 || p.Total != 3 || p.Next != "p2" || p.Threads[0].Comments[0].Body != "hm" {
		t.Fatalf("page = %+v, want the first page with its comments", p)
	}
	if again := readThreads(t, s, threadsQ); again.Next != p.Next || len(again.Threads) != 2 {
		t.Errorf("second read = %+v, want %+v", again, p)
	}
	if n := len(r.requests()); n != 1 {
		t.Errorf("requests = %d, want 1: a fresh page is a hit", n)
	}
	if c, ok := s.CachedThreads(threadsQ); !ok || len(c.Threads) != 2 {
		t.Errorf("CachedThreads after a read = %+v, %v, want the page", c, ok)
	}
}

func TestThreadsNeedHead(t *testing.T) {
	r := &threadServer{}
	s := New(r.api())
	q := threadsQ
	q.Head = ""
	if _, err := s.Threads(t.Context(), q); !errors.Is(err, errNoHead) {
		t.Errorf("Threads without a head = %v, want errNoHead", err)
	}
	if _, ok := s.CachedThreads(q); ok {
		t.Error("CachedThreads without a head = true, want false")
	}
	if n := len(r.requests()); n != 0 {
		t.Errorf("requests = %d, want none", n)
	}
}

func TestThreadsPageSize(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		q    int
		want int
	}{
		{"the default of the config", nil, 0, 50},
		{"the configured size", []Option{WithDetailSizes(github.DetailSizes{Threads: 20})}, 0, 20},
		{"the query's size", []Option{WithDetailSizes(github.DetailSizes{Threads: 20})}, 7, 7},
		{"at most 100", nil, 500, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &threadServer{}
			q := threadsQ
			q.PageSize = tt.q
			readThreads(t, New(r.api(), tt.opts...), q)
			if got := r.requests(); len(got) != 1 || got[0].first != tt.want {
				t.Errorf("requests = %+v, want one of size %d", got, tt.want)
			}
		})
	}
}

func TestThreadsMorePagesOnRequest(t *testing.T) {
	r := &threadServer{}
	s := New(r.api())
	first := readThreads(t, s, threadsQ)
	if got := r.requests(); len(got) != 1 || got[0].cursor != "" {
		t.Fatalf("requests = %+v, want only the first page, which has more", got)
	}
	if _, ok := s.CachedThreads(ThreadsQuery{Repo: repo, Number: 1, Head: "abc123", Cursor: first.Next}); ok {
		t.Fatal("CachedThreads of the second page before it was read = true, want false")
	}

	q := threadsQ
	q.Cursor = first.Next
	second := readThreads(t, s, q)
	if !second.Last() || len(second.Threads) != 1 || second.Threads[0].ID != "t3" {
		t.Fatalf("second page = %+v, want the last, with t3", second)
	}
	if got := r.requests(); len(got) != 2 || got[1] != (threadRead{"p2", 50}) {
		t.Errorf("requests = %+v, want the second asked with the cursor and the size", got)
	}
	// Each page is cached on its own.
	readThreads(t, s, threadsQ)
	readThreads(t, s, q)
	if n := len(r.requests()); n != 2 {
		t.Errorf("requests = %d, want 2: both pages are cached", n)
	}
}

func TestThreadsNewHeadIsNewKey(t *testing.T) {
	r := &threadServer{}
	s := New(r.api())
	readThreads(t, s, threadsQ)
	moved := threadsQ
	moved.Head = "def456"
	if _, ok := s.CachedThreads(moved); ok {
		t.Fatal("CachedThreads of a new head = true, want false")
	}
	readThreads(t, s, moved)
	if n := len(r.requests()); n != 2 {
		t.Errorf("requests = %d, want 2: a push reads its threads anew", n)
	}
	// The head is compared as GitHub writes it, whatever its case.
	upper := threadsQ
	upper.Head = "ABC123"
	readThreads(t, s, upper)
	if n := len(r.requests()); n != 2 {
		t.Errorf("requests = %d, want the same head in upper case to hit", n)
	}
}

func TestThreadsStaleIsReadAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &threadServer{}
		s := New(r.api(), WithTTL(time.Minute))
		readThreads(t, s, threadsQ)
		time.Sleep(2 * time.Minute)
		if _, ok := s.CachedThreads(threadsQ); !ok {
			t.Error("CachedThreads of a stale page = false, want it served")
		}
		readThreads(t, s, threadsQ)
		if n := len(r.requests()); n != 2 {
			t.Errorf("requests = %d, want the stale page read again in full", n)
		}
	})
}

func TestThreadsInvalidate(t *testing.T) {
	r := &threadServer{}
	s := New(r.api())
	readThreads(t, s, threadsQ)
	s.Invalidate(repo)
	if _, ok := s.CachedThreads(threadsQ); !ok {
		t.Error("CachedThreads after Invalidate = false, want the stale page")
	}
	readThreads(t, s, threadsQ)
	if n := len(r.requests()); n != 2 {
		t.Errorf("requests = %d, want a read after Invalidate", n)
	}
}

func TestThreadsKeptIsReadAgainInANewSession(t *testing.T) {
	r := &threadServer{}
	store := openStore(t)
	readThreads(t, New(r.api(), WithStore(cachetest.Aged(store, time.Hour))), threadsQ)

	s := New(r.api(), WithStore(cachetest.Aged(store, time.Hour)))
	r.fail(errDial)
	p := readThreads(t, s, threadsQ)
	if !p.Offline || len(p.Threads) != 2 {
		t.Errorf("page = Offline %v, %d threads; want the kept page, offline", p.Offline, len(p.Threads))
	}

	r.fail(fmt.Errorf("graphql: %w", &core.RateLimitError{Reset: epoch}))
	s = New(r.api(), WithStore(cachetest.Aged(store, time.Hour)))
	if p := readThreads(t, s, threadsQ); !p.Limited || p.Offline {
		t.Errorf("page = Offline %v, Limited %v; want the kept page, limited", p.Offline, p.Limited)
	}

	r.fail(nil)
	s = New(r.api(), WithStore(cachetest.Aged(store, time.Hour)))
	before := len(r.requests())
	if p := readThreads(t, s, threadsQ); p.Offline || p.Limited || len(r.requests()) != before+1 {
		t.Errorf("page = %+v after %d requests, want the stale kept page read again", p, len(r.requests())-before)
	}
}

func TestThreadsNotFoundHasNoPage(t *testing.T) {
	r := &threadServer{}
	r.fail(fmt.Errorf("graphql: %w", core.ErrNotFound))
	s := New(r.api())
	if _, err := s.Threads(t.Context(), threadsQ); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Threads = %v, want not found", err)
	}
	if _, ok := s.CachedThreads(threadsQ); ok {
		t.Error("a failed read was cached")
	}
}

func TestSetThreadResolved(t *testing.T) {
	r := &threadServer{}
	s := New(r.api())
	read := readThreads(t, s, threadsQ)
	second := threadsQ
	second.Cursor = read.Next
	readThreads(t, s, second)

	rollback, ok := s.SetThreadResolved(repo, 1, "t2", true)
	if !ok {
		t.Fatal("SetThreadResolved of a cached thread = false, want true")
	}
	got, _ := s.CachedThreads(threadsQ)
	if t1, t2 := got.Threads[0], got.Threads[1]; t1.Resolved || !t2.Resolved || t2.ViewerCanResolve || !t2.ViewerCanUnresolve {
		t.Errorf("threads = %+v, %+v; want only t2 resolved, which the viewer may only unresolve now", t1, t2)
	}
	if read.Threads[1].Resolved {
		t.Error("the page that was returned before the change was changed")
	}
	// It reads nothing, and the page is still the cached one.
	if p := readThreads(t, s, threadsQ); !p.Threads[1].Resolved || len(r.requests()) != 2 {
		t.Errorf("read after the change = %+v after %d requests, want the changed page from the cache", p.Threads[1], len(r.requests()))
	}

	rollback()
	got, _ = s.CachedThreads(threadsQ)
	if got.Threads[1].Resolved || !got.Threads[1].ViewerCanResolve {
		t.Errorf("thread after the rollback = %+v, want it as it was read", got.Threads[1])
	}

	// A thread on a page after the first is found as well, and a resolved one
	// is unresolved.
	if _, ok := s.SetThreadResolved(repo, 1, "t3", true); !ok {
		t.Fatal("SetThreadResolved of a thread of the second page = false, want true")
	}
	if _, ok := s.SetThreadResolved(repo, 1, "t3", false); !ok {
		t.Fatal("SetThreadResolved to unresolve = false, want true")
	}
	if p, _ := s.CachedThreads(second); p.Threads[0].Resolved || !p.Threads[0].ViewerCanResolve {
		t.Errorf("thread = %+v, want it unresolved", p.Threads[0])
	}
}

// resolvedIn reports whether thread t2 of p is resolved.
func resolvedIn(p core.ThreadPage) bool { return p.Threads[1].Resolved }

func TestConfirmThreadResolvedIsKept(t *testing.T) {
	r := &threadServer{}
	store := openStore(t)
	s := New(r.api(), WithStore(store))
	readThreads(t, s, threadsQ)
	if _, ok := s.SetThreadResolved(repo, 1, "t2", true); !ok {
		t.Fatal("SetThreadResolved = false, want true")
	}
	s.ConfirmThreadResolved(repo, 1, "t2", true)

	// A new session, within the TTL, serves the page kept with the change and
	// reads nothing.
	s = New(r.api(), WithStore(store))
	p := readThreads(t, s, threadsQ)
	if !resolvedIn(p) || p.Threads[0].Resolved {
		t.Errorf("kept page = %+v, %+v; want only t2 resolved", p.Threads[0], p.Threads[1])
	}
	if n := len(r.requests()); n != 1 {
		t.Errorf("requests = %d, want 1: the kept page is fresh and holds the change", n)
	}
}

func TestConfirmThreadResolvedStaysStale(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &threadServer{}
		store := openStore(t)
		s := New(r.api(), WithTTL(time.Minute), WithStore(store))
		readThreads(t, s, threadsQ)
		time.Sleep(2 * time.Minute)
		s.SetThreadResolved(repo, 1, "t2", true)
		s.ConfirmThreadResolved(repo, 1, "t2", true)

		// The page was read two minutes ago, so confirming the change doesn't
		// make it fresh: it is read again.
		s = New(r.api(), WithTTL(time.Minute), WithStore(store))
		readThreads(t, s, threadsQ)
		if n := len(r.requests()); n != 2 {
			t.Errorf("requests = %d, want the old kept page read again", n)
		}
	})
}

// detailWithThreads is a detail that lists thread t2 as unresolved, among one
// resolved and one other unresolved thread.
func detailWithThreads(_ context.Context, _ core.RepoRef, number int) (core.PullRequestDetail, error) {
	d, _ := detail(context.TODO(), repo, number)
	d.HeadSHA = "abc123"
	d.Threads = core.ThreadSummary{
		Total: 3, Unresolved: 2, Resolved: 1,
		Threads: []core.ReviewThread{{ID: "t1"}, {ID: "t2"}, {ID: "t3", Resolved: true}},
	}
	return d, nil
}

func TestResolveThreadUpdatesCachesAndKeepsIt(t *testing.T) {
	r := &threadServer{}
	api := r.api()
	api.get = detailWithThreads
	store := openStore(t)
	s := New(api, WithStore(store))
	readThreads(t, s, threadsQ)
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}

	op := s.ResolveThread(repo, 1, "t2", true)
	if p, _ := s.CachedThreads(threadsQ); !resolvedIn(p) {
		t.Error("the cached page doesn't show the thread resolved at once")
	}
	if d, _ := s.CachedGet(repo, 1); d.Threads.Unresolved != 1 || d.Threads.Resolved != 2 || !d.Threads.Threads[1].Resolved {
		t.Errorf("detail threads = %+v, want one unresolved and two resolved", d.Threads)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if n := api.count("thread t2 true"); n != 1 {
		t.Errorf("resolve requests = %d, want 1", n)
	}

	// The next session serves both kept with the change.
	api2 := r.api()
	api2.get = detailWithThreads
	s = New(api2, WithStore(store))
	if p := readThreads(t, s, threadsQ); !resolvedIn(p) {
		t.Error("kept page doesn't hold the confirmed change")
	}
	if d, err := s.Get(t.Context(), repo, 1); err != nil || d.Threads.Unresolved != 1 {
		t.Errorf("kept detail = %+v, %v; want one unresolved thread", d.Threads, err)
	}
}

func TestResolveThreadRollsBackWhenRefused(t *testing.T) {
	r := &threadServer{}
	api := r.api()
	api.get = detailWithThreads
	api.threadSetErr = errors.New("nope")
	s := New(api)
	readThreads(t, s, threadsQ)
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	op := s.ResolveThread(repo, 1, "t3", false)
	if err := op.Do(t.Context()); err == nil {
		t.Fatal("Do = nil, want the refusal")
	}
	if d, _ := s.CachedGet(repo, 1); d.Threads.Unresolved != 2 || !d.Threads.Threads[2].Resolved {
		t.Errorf("detail threads = %+v, want them as read", d.Threads)
	}
}

func TestSetThreadResolvedRollbackKeepsNothing(t *testing.T) {
	r := &threadServer{}
	store := openStore(t)
	s := New(r.api(), WithStore(store))
	readThreads(t, s, threadsQ)
	rollback, _ := s.SetThreadResolved(repo, 1, "t2", true)
	rollback()

	// GitHub refused it and nothing confirmed it, so what was kept is what was
	// read.
	s = New(r.api(), WithStore(store))
	if p := readThreads(t, s, threadsQ); resolvedIn(p) || len(r.requests()) != 1 {
		t.Errorf("kept page = %+v after %d requests, want t2 unresolved, as read", p.Threads[1], len(r.requests()))
	}
	// Confirming a change that was never made keeps nothing either.
	s.ConfirmThreadResolved(repo, 1, "t2", true)
	s = New(r.api(), WithStore(store))
	if p := readThreads(t, s, threadsQ); resolvedIn(p) {
		t.Error("a confirm of a change that isn't held kept it")
	}
}

func TestSetThreadResolvedDuringRead(t *testing.T) {
	gate := make(chan struct{}, 1)
	started := make(chan struct{}, 1)
	r := &threadServer{}
	api := &fakeAPI{threads: func(ctx context.Context, rp core.RepoRef, n int, cursor string, first int) (core.ThreadPage, error) {
		started <- struct{}{}
		<-gate
		return r.serve(ctx, rp, n, cursor, first)
	}}
	s := New(api)
	gate <- struct{}{} // the first read goes through at once
	readThreads(t, s, threadsQ)
	<-started

	// A read again is in flight when the change is made, and brings the page
	// as it was before.
	s.Invalidate(repo)
	done := make(chan core.ThreadPage)
	go func() { done <- readThreads(t, s, threadsQ) }()
	<-started
	rollback, ok := s.SetThreadResolved(repo, 1, "t2", true)
	if !ok {
		t.Fatal("SetThreadResolved of a cached, stale page = false, want true")
	}
	gate <- struct{}{}
	<-done

	if p, _ := s.CachedThreads(threadsQ); !resolvedIn(p) {
		t.Error("the read that was in flight undid the change")
	}
	rollback()
	if p, _ := s.CachedThreads(threadsQ); resolvedIn(p) {
		t.Error("the rollback didn't restore the page")
	}
}

func TestRevalidateReadsThreadsAgain(t *testing.T) {
	r := &threadServer{}
	api := r.api()
	api.get = func(context.Context, core.RepoRef, int) (core.PullRequestDetail, error) {
		var d core.PullRequestDetail
		d.Number = 1
		return d, nil
	}
	s := New(api)
	readThreads(t, s, threadsQ)
	if _, err := s.Revalidate(t.Context(), repo, 1); err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	readThreads(t, s, threadsQ)
	if n := len(r.requests()); n != 2 {
		t.Errorf("thread requests = %d, want the page read again after the detail was", n)
	}
}

func TestSetThreadResolvedUnknown(t *testing.T) {
	r := &threadServer{}
	s := New(r.api())
	if _, ok := s.SetThreadResolved(repo, 1, "t1", true); ok {
		t.Error("SetThreadResolved with nothing cached = true, want false")
	}
	readThreads(t, s, threadsQ)
	for name, call := range map[string]func() bool{
		"a thread that isn't cached": func() bool { _, ok := s.SetThreadResolved(repo, 1, "nope", true); return ok },
		"a thread of another pull":   func() bool { _, ok := s.SetThreadResolved(repo, 2, "t1", true); return ok },
		"a thread of another repository": func() bool {
			_, ok := s.SetThreadResolved(core.RepoRef{Owner: "a", Name: "b"}, 1, "t1", true)
			return ok
		},
	} {
		if call() {
			t.Errorf("SetThreadResolved of %s = true, want false", name)
		}
	}
	if p, _ := s.CachedThreads(threadsQ); p.Threads[0].Resolved {
		t.Error("a change that found no thread changed one")
	}
}
