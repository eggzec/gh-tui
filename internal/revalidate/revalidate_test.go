package revalidate

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

var (
	repoA = core.RepoRef{Owner: "octo", Name: "a"}
	repoB = core.RepoRef{Owner: "octo", Name: "b"}
	repoC = core.RepoRef{Owner: "octo", Name: "c"}
)

// server is a fake GitHub that the checks of its entries ask. result
// decides what the nth check of an entry (starting at 0) finds; nil means
// not modified. Each check takes latency.
type server struct {
	start   time.Time
	latency time.Duration
	result  func(id string, n int) Result

	mu    sync.Mutex
	calls []call
	byID  map[string]int
}

type call struct {
	id string
	at time.Duration
}

func newServer(result func(id string, n int) Result) *server {
	return &server{start: time.Now(), latency: 100 * time.Millisecond, result: result, byID: make(map[string]int)}
}

// entry returns an entry of repo, last used ago before now.
func (s *server) entry(id string, repo core.RepoRef, ago time.Duration) Entry {
	return Entry{
		ID:     id,
		Repo:   repo,
		UsedAt: time.Now().Add(-ago),
		Check: func(ctx context.Context) Result {
			s.mu.Lock()
			n := s.byID[id]
			s.byID[id]++
			s.calls = append(s.calls, call{id: id, at: time.Since(s.start)})
			s.mu.Unlock()
			select {
			case <-time.After(s.latency):
			case <-ctx.Done():
				return Result{Status: Failed, Err: ctx.Err()}
			}
			if s.result == nil {
				return Result{Status: NotModified}
			}
			return s.result(id, n)
		},
	}
}

func (s *server) ids() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.calls))
	for _, c := range s.calls {
		ids = append(ids, c.id)
	}
	return ids
}

func (s *server) times() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts := make([]time.Duration, 0, len(s.calls))
	for _, c := range s.calls {
		ts = append(ts, c.at)
	}
	return ts
}

// recorder collects what the revalidator publishes and reports.
type recorder struct {
	mu        sync.Mutex
	published []string
	passes    []Pass
}

func (r *recorder) publish(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.published = append(r.published, key)
}

func (r *recorder) report(p Pass) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.passes = append(r.passes, p)
}

func (r *recorder) keys() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.published)
}

func (r *recorder) all() []Pass {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.passes)
}

// start runs a revalidator of entries, reporting to rec, until the test
// ends.
func start(t *testing.T, entries []Entry, rec *recorder, opts ...Option) *Revalidator {
	t.Helper()
	opts = append([]Option{WithStartDelay(0), WithPublish(rec.publish), WithReport(rec.report)}, opts...)
	r := New([]Source{func() []Entry { return entries }}, opts...)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Errorf("Run = %v, want %v", err, context.Canceled)
		}
	})
	return r
}

func TestBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := newServer(nil)
		entries := make([]Entry, 0, 300)
		for i := range 300 {
			entries = append(entries, srv.entry(string(rune('a'+i%26))+string(rune('0'+i/26)), repoB, time.Duration(i)*time.Minute))
		}
		rec := new(recorder)
		start(t, entries, rec, WithBudget(60), WithInterval(2*time.Minute), WithFreshFor(time.Hour))
		synctest.Sleep(20 * time.Minute)

		times := srv.times()
		for i := range times {
			j := i
			for j < len(times) && times[j]-times[i] < time.Minute {
				j++
			}
			if j-i > 60 {
				t.Fatalf("%d requests in the minute from %v, want at most 60", j-i, times[i])
			}
		}
		passes := rec.all()
		if len(passes) < 2 {
			t.Fatalf("%d passes in 20 minutes, want several", len(passes))
		}
		p := passes[0]
		if p.Due != 300 || p.Sent != 120 || p.NotModified != 120 || p.Deferred != 180 {
			t.Errorf("first pass = %+v, want 120 of 300 due sent and the rest deferred", p)
		}
		// Each entry is checked once while fresh, so every one gets its
		// turn over the later passes.
		if got := len(srv.ids()); got != 300 {
			t.Errorf("%d checks in 20 minutes, want every entry once", got)
		}
		if got := rec.keys(); len(got) != 0 {
			t.Errorf("published %q for 304s, want nothing", got)
		}
	})
}

func TestPriority(t *testing.T) {
	for _, tt := range []struct {
		scope Scope
		want  []string
	}{
		{ScopeRecent, []string{"a-new", "inbox", "a-old", "b-new", "c-mid", "b-old"}},
		{ScopeAll, []string{"a-new", "inbox", "inbox-old", "a-old", "b-new", "c-mid", "b-old", "c-ancient"}},
	} {
		t.Run(map[Scope]string{ScopeRecent: "recent", ScopeAll: "all"}[tt.scope], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				srv := newServer(nil)
				fresh := srv.entry("a-fresh", repoA, 0)
				fresh.CheckedAt = time.Now().Add(-time.Minute)
				entries := []Entry{
					srv.entry("b-old", repoB, 3*24*time.Hour),
					srv.entry("c-ancient", repoC, 30*24*time.Hour),
					srv.entry("a-old", core.RepoRef{Owner: "OCTO", Name: "A"}, 40*24*time.Hour),
					srv.entry("inbox", core.RepoRef{}, time.Hour),
					srv.entry("inbox-old", core.RepoRef{}, 30*24*time.Hour),
					srv.entry("b-new", repoB, 2*time.Hour),
					srv.entry("a-new", repoA, time.Minute),
					srv.entry("c-mid", repoC, 24*time.Hour),
					fresh,
				}
				rec := new(recorder)
				r := New([]Source{func() []Entry { return entries }}, WithStartDelay(0), WithScope(tt.scope),
					WithConcurrency(1), WithFreshFor(5*time.Minute), WithReport(rec.report))
				r.SetRepo(repoA)
				ctx, cancel := context.WithCancel(t.Context())
				go func() { _ = r.Run(ctx) }()
				synctest.Sleep(time.Minute)
				cancel()
				synctest.Wait()
				if got := srv.ids(); !slices.Equal(got, tt.want) {
					t.Errorf("checked %q, want %q", got, tt.want)
				}
			})
		})
	}
}

func TestChangesArePublishedOncePerGroup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		changed := map[string]string{
			"a1": "issues:a", "a2": "issues:a", "a3": "issues:a", "a4": "files:a",
			"b1": "issues:b", "b2": "issues:b",
		}
		srv := newServer(func(id string, n int) Result {
			if key, ok := changed[id]; ok && n == 0 {
				return Result{Status: Changed, Sync: key}
			}
			return Result{Status: NotModified}
		})
		entries := make([]Entry, 0, 7)
		for _, id := range []string{"a1", "a2", "a3", "a4", "a5"} {
			entries = append(entries, srv.entry(id, repoA, time.Minute))
		}
		entries = append(entries, srv.entry("b1", repoB, time.Hour), srv.entry("b2", repoB, time.Hour))
		rec := new(recorder)
		r := start(t, entries, rec, WithStartDelay(time.Second))
		r.SetRepo(repoA)
		synctest.Sleep(time.Minute)

		want := []string{"files:a", "issues:a", "issues:b"}
		if got := rec.keys(); !slices.Equal(got, want) {
			t.Errorf("published %q, want %q: each key once, the selected repository's first", got, want)
		}
		if p := rec.all()[0]; p.Changed != 6 || p.NotModified != 1 || !slices.Equal(p.Published, want) {
			t.Errorf("pass = %+v, want 6 changed, 1 not modified", p)
		}
	})
}

func TestSkippedIsFree(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := newServer(func(id string, _ int) Result {
			if id[0] == 's' {
				return Result{Status: Skipped}
			}
			return Result{Status: NotModified}
		})
		entries := make([]Entry, 0, 7)
		// Entries used equally long ago go by ID, so the skipped ones
		// come first and would use up the budget if they cost any.
		for _, id := range []string{"s1", "s2", "s3", "s4", "s5", "t1", "t2"} {
			entries = append(entries, srv.entry(id, repoA, time.Minute))
		}
		rec := new(recorder)
		start(t, entries, rec, WithBudget(2), WithInterval(4*time.Minute))
		synctest.Sleep(10 * time.Second)
		if got := len(srv.ids()); got != 7 {
			t.Errorf("%d checks in 10s under a budget of 2, want all 7: skipped ones cost nothing", got)
		}
		if p := rec.all(); len(p) != 1 || p[0].Sent != 2 || p[0].Skipped != 5 {
			t.Errorf("passes = %+v, want 2 sent and 5 skipped", p)
		}
	})
}

func TestOfflinePausesAndBacksOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		offline := true
		srv := newServer(func(string, int) Result {
			mu.Lock()
			defer mu.Unlock()
			if offline {
				return Result{Status: Offline, Err: errors.New("dial tcp: no route to host")}
			}
			return Result{Status: NotModified}
		})
		entries := []Entry{srv.entry("a", repoA, time.Minute), srv.entry("b", repoA, 2*time.Minute), srv.entry("c", repoA, 3*time.Minute)}
		rec := new(recorder)
		start(t, entries, rec, WithConcurrency(1), WithInterval(time.Minute), WithFreshFor(time.Hour))

		// Passes at 0, then 2m and 4m after each ended as the backoff
		// doubles.
		synctest.Sleep(7 * time.Minute)
		if got := srv.ids(); !slices.Equal(got, []string{"a", "a", "a"}) {
			t.Errorf("checked %q while offline, want only the first entry, once a pass", got)
		}
		passes := rec.all()
		if len(passes) != 3 || !passes[0].Offline || passes[0].Deferred != 2 {
			t.Fatalf("passes = %+v, want 3 that stopped offline, deferring 2", passes)
		}
		lat := srv.latency
		want := []time.Duration{0, 2*time.Minute + lat, 6*time.Minute + 2*lat}
		for i, p := range passes {
			if at := p.Start.Sub(srv.start); at != want[i] {
				t.Errorf("pass %d at %v, want %v", i, at, want[i])
			}
		}

		mu.Lock()
		offline = false
		mu.Unlock()
		// The next pass, 8m after the last, finds GitHub again.
		synctest.Sleep(8 * time.Minute)
		if got := srv.ids()[3:]; !slices.Equal(got, []string{"a", "b", "c"}) {
			t.Errorf("checked %q back online, want every entry", got)
		}
	})
}

func TestRateLimitWaitsForReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(15 * time.Minute)
		srv := newServer(func(string, int) Result {
			if time.Now().Before(reset) {
				return Result{Status: Limited, RetryAt: reset, Err: &core.RateLimitError{Reset: reset}}
			}
			return Result{Status: NotModified}
		})
		entries := []Entry{srv.entry("a", repoA, time.Minute), srv.entry("b", repoB, time.Minute)}
		rec := new(recorder)
		r := start(t, entries, rec, WithConcurrency(1), WithInterval(time.Minute), WithFreshFor(time.Hour))
		synctest.Sleep(time.Minute)
		// Selecting a repository doesn't lift the limit.
		r.SetRepo(repoB)
		synctest.Sleep(13 * time.Minute)
		if got := srv.times(); len(got) != 1 {
			t.Errorf("checks at %v before the reset, want one", got)
		}
		synctest.Sleep(2 * time.Minute)
		if got := srv.times(); len(got) != 3 || got[1] != 15*time.Minute {
			t.Errorf("checks at %v, want the next ones at the reset, 15m", got)
		}
		if p := rec.all()[0]; !p.RetryAt.Equal(reset) || p.Deferred != 1 {
			t.Errorf("first pass = %+v, want it limited until the reset", p)
		}
	})
}

func TestInactiveSlowsDown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := newServer(nil)
		srv.latency = 0
		entries := make([]Entry, 0, 10)
		for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
			entries = append(entries, srv.entry(id, repoA, time.Minute))
		}
		rec := new(recorder)
		r := New([]Source{func() []Entry { return entries }}, WithStartDelay(0), WithBudget(8),
			WithInterval(time.Minute), WithIdleMultiplier(4), WithFreshFor(time.Second), WithReport(rec.report))
		r.SetActive(false)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go func() { _ = r.Run(ctx) }()

		// Inactive, the budget is 2 a minute: the first pass's 8 checks
		// take 4 minutes.
		synctest.Sleep(3*time.Minute + 30*time.Second)
		if got := len(srv.ids()); got != 8 {
			t.Errorf("%d checks in 3.5 minutes while inactive, want 8 at 2 a minute", got)
		}
		// The pass ended at 3m, and the next one waits 4 intervals.
		synctest.Sleep(3*time.Minute + 20*time.Second)
		if got := len(rec.all()); got != 1 {
			t.Fatalf("%d passes before 4 intervals, want 1", got)
		}
		r.SetActive(true)
		synctest.Sleep(20 * time.Second)
		if got := len(rec.all()); got != 2 {
			t.Errorf("%d passes after 4 intervals, want 2", got)
		}
		cancel()
		synctest.Wait()
	})
}

func TestSetRepoStartsPass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := newServer(nil)
		entries := []Entry{srv.entry("a", repoA, time.Minute), srv.entry("b", repoB, 30*24*time.Hour)}
		rec := new(recorder)
		r := start(t, entries, rec, WithInterval(10*time.Minute), WithFreshFor(time.Hour))
		r.SetRepo(repoA)
		synctest.Sleep(time.Minute)
		if got := srv.ids(); !slices.Equal(got, []string{"a"}) {
			t.Fatalf("checked %q, want the recent entry", got)
		}
		r.SetRepo(repoA)
		r.SetRepo(repoB)
		synctest.Sleep(time.Minute)
		if got := srv.ids(); !slices.Equal(got, []string{"a", "b"}) {
			t.Errorf("checked %q after selecting b, want b's old entry at once", got)
		}
	})
}

func TestRunOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := New(nil)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- r.Run(ctx) }()
		synctest.Wait()
		if err := r.Run(t.Context()); !errors.Is(err, ErrRan) {
			t.Errorf("second Run = %v, want %v", err, ErrRan)
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Errorf("Run = %v, want %v", err, context.Canceled)
		}
	})
}

func TestStatusString(t *testing.T) {
	for s, want := range map[Status]string{
		Skipped: "skipped", NotModified: "not modified", Changed: "changed", Gone: "gone",
		Offline: "offline", Limited: "limited", Failed: "failed", Status(99): "unknown",
	} {
		if got := s.String(); got != want {
			t.Errorf("Status(%d) = %q, want %q", int(s), got, want)
		}
	}
}
