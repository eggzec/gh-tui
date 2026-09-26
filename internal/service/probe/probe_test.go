package probe

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/watch"
)

// server answers probes like GitHub: 304 when cond has the current ETag,
// else 200 with it. It records the conds it saw.
type server struct {
	mu    sync.Mutex
	etag  string
	err   error
	conds []github.Conditional
}

func (s *server) probe(_ context.Context, cond github.Conditional) (github.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conds = append(s.conds, cond)
	if s.err != nil {
		return github.Response{}, s.err
	}
	res := github.Response{PollInterval: time.Minute}
	if cond.ETag != "" && cond.ETag == s.etag {
		res.NotModified = true
		return res, nil
	}
	res.ETag = s.etag
	return res, nil
}

func (s *server) set(etag string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.etag, s.err = etag, err
}

func TestPoll(t *testing.T) {
	srv := &server{etag: `"e1"`}
	var tr Tracker
	changes := 0
	poll := tr.Poll("k", srv.probe, func() { changes++ })

	steps := []struct {
		name    string
		etag    string
		err     error
		want    watch.Result
		wantErr bool
		cond    string
	}{
		{name: "first only records", etag: `"e1"`, want: watch.Result{Interval: time.Minute}},
		{name: "304 is no change", etag: `"e1"`, want: watch.Result{Interval: time.Minute}, cond: `"e1"`},
		{name: "new ETag is a change", etag: `"e2"`, want: watch.Result{Changed: true, Interval: time.Minute}, cond: `"e1"`},
		{name: "error keeps the ETag", err: errors.New("boom"), wantErr: true, cond: `"e2"`},
		{name: "after the error", etag: `"e2"`, want: watch.Result{Interval: time.Minute}, cond: `"e2"`},
		{name: "missing ETag is no change", etag: "", want: watch.Result{Interval: time.Minute}, cond: `"e2"`},
	}
	for i, st := range steps {
		srv.set(st.etag, st.err)
		got, err := poll(t.Context())
		if (err != nil) != st.wantErr {
			t.Fatalf("%s: error = %v, wantErr %v", st.name, err, st.wantErr)
		}
		if got != st.want {
			t.Errorf("%s: result = %+v, want %+v", st.name, got, st.want)
		}
		if c := srv.conds[i].ETag; c != st.cond {
			t.Errorf("%s: sent ETag %q, want %q", st.name, c, st.cond)
		}
	}
	if changes != 1 {
		t.Errorf("changed was called %d times, want 1", changes)
	}
}

func TestPollKeysApart(t *testing.T) {
	a, b := &server{etag: `"a"`}, &server{etag: `"b"`}
	var tr Tracker
	pa := tr.Poll("a", a.probe, func() { t.Error("a changed") })
	pb := tr.Poll("b", b.probe, func() { t.Error("b changed") })
	for _, poll := range []watch.PollFunc{pa, pb, pa, pb} {
		if _, err := poll(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if a.conds[1].ETag != `"a"` || b.conds[1].ETag != `"b"` {
		t.Errorf("conds = %v, %v; want each key's own ETag", a.conds, b.conds)
	}
}

func TestPollConcurrent(t *testing.T) {
	srv := &server{etag: `"e1"`}
	var tr Tracker
	var mu sync.Mutex
	changes := 0
	var wg sync.WaitGroup
	for range 8 {
		poll := tr.Poll("k", srv.probe, func() { mu.Lock(); changes++; mu.Unlock() })
		wg.Go(func() {
			for range 10 {
				if _, err := poll(t.Context()); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	if changes != 0 {
		t.Errorf("%d changes without a new ETag, want 0", changes)
	}
}

func TestPollKeepsETags(t *testing.T) {
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := &server{etag: `"e1"`}
	var first Tracker
	first.Keep(store)
	if _, err := first.Poll("k", srv.probe, func() {})(t.Context()); err != nil {
		t.Fatal(err)
	}

	// The next session asks with the kept ETag, so nothing changed is a
	// free 304, and a change made in between is reported at once.
	for _, tt := range []struct {
		etag    string
		changed bool
		cond    string
	}{
		{`"e1"`, false, `"e1"`},
		{`"e2"`, true, `"e1"`},
		{`"e2"`, false, `"e2"`},
	} {
		srv.set(tt.etag, nil)
		var next Tracker
		next.Keep(store)
		changes := 0
		res, err := next.Poll("k", srv.probe, func() { changes++ })(t.Context())
		if err != nil || res.Changed != tt.changed || changes != btoi(tt.changed) {
			t.Errorf("first poll with %s = %+v, %v, %d changes; want changed %v", tt.etag, res, err, changes, tt.changed)
		}
		if got := srv.conds[len(srv.conds)-1].ETag; got != tt.cond {
			t.Errorf("asked with %s, want %s", got, tt.cond)
		}
	}
	var other Tracker
	other.Keep(store)
	if _, err := other.Poll("other", srv.probe, func() { t.Error("changed on the first poll of a key never kept") })(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestETag(t *testing.T) {
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := &server{etag: `"e1"`}
	var first Tracker
	first.Keep(store)
	if got := first.ETag("k"); got != "" {
		t.Errorf("ETag before any probe = %q, want none", got)
	}
	if _, err := first.Poll("k", srv.probe, func() {})(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := first.ETag("k"); got != `"e1"` {
		t.Errorf("ETag after a probe = %q, want the probe's", got)
	}
	var next Tracker
	next.Keep(store)
	if got := next.ETag("k"); got != `"e1"` {
		t.Errorf("ETag in a new session = %q, want the kept one", got)
	}
	if got := next.ETag("other"); got != "" {
		t.Errorf("ETag of a key never probed = %q, want none", got)
	}
}
