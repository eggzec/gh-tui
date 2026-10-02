package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// aheadWorkers is the most rows an Ahead reads at once while it shares
// no slots.
var aheadWorkers = config.Default().Prefetch.Parallel

// reader is a fake detail read that records the rows read, and can hold
// the reads until released or fail them.
type reader struct {
	mu       sync.Mutex
	cached   map[int]bool
	read     []int
	inFlight int
	most     int
	// hold, if set, holds each read until it is closed or the read is
	// cancelled.
	hold      chan struct{}
	cancelled []int
	err       error
}

func newReader() *reader { return &reader{cached: map[int]bool{}} }

func (r *reader) readRow(ctx context.Context, k int) error {
	r.mu.Lock()
	r.read = append(r.read, k)
	r.inFlight++
	r.most = max(r.most, r.inFlight)
	hold, err := r.hold, r.err
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.inFlight--
		r.mu.Unlock()
	}()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			r.mu.Lock()
			r.cancelled = append(r.cancelled, k)
			r.mu.Unlock()
			return ctx.Err()
		}
	}
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.cached[k] = true
	r.mu.Unlock()
	return nil
}

func (r *reader) current(k int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cached[k]
}

func (r *reader) reads() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.read)
	slices.Sort(out)
	return out
}

// rowsOf returns the rows of a list of n loaded rows, numbered from 1.
func rowsOf(n int) func(int) (int, bool) {
	return func(i int) (int, bool) { return i + 1, i < n }
}

// run runs cmd, and the commands of a batch it returns, to the end.
func run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if b, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range b {
			run(c)
		}
	}
}

// aheadOf returns an Ahead of rows that read reads and current reports
// cached, reset for a list, whose window holds the before rows above the
// cursor and the after rows below it, read once the cursor rests for rest.
func aheadOf(t *testing.T, read func(context.Context, int) error, current func(int) bool, before, after int, rest time.Duration) *Ahead[int] {
	t.Helper()
	a := NewAhead("row", read, current)
	a.Configure(config.Resolved{Enabled: true, Window: config.Window{Before: before, After: after}, Rest: rest})
	a.Reset(t.Context())
	return a
}

// newAhead returns an Ahead of the rows r reads, as aheadOf does.
func newAhead(t *testing.T, r *reader, before, after int, rest time.Duration) *Ahead[int] {
	t.Helper()
	return aheadOf(t, r.readRow, r.current, before, after, rest)
}

// win sets the window of a to before and after, with the rest it has,
// and returns it.
func win(a *Ahead[int], before, after int) *Ahead[int] {
	a.Configure(config.Resolved{Enabled: true, Window: config.Window{Before: before, After: after}, Rest: a.delay})
	return a
}

// rest runs the delay cmd started, and passes its message to a.
func rest(t *testing.T, a *Ahead[int], cmd tea.Cmd) tea.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatal("no delay started")
	}
	msg, ok := cmd().(AheadMsg)
	if !ok {
		t.Fatal("the delay didn't report an AheadMsg")
	}
	return a.Rested(msg)
}

func TestAheadBoundsReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := newAhead(t, r, 0, 9, time.Millisecond)
		cmd := a.Window(rowsOf(10), 0)
		done := make(chan struct{})
		go func() {
			run(cmd)
			close(done)
		}()
		synctest.Wait()
		r.mu.Lock()
		if r.inFlight != aheadWorkers {
			t.Errorf("%d reads in flight, want %d", r.inFlight, aheadWorkers)
		}
		r.mu.Unlock()
		close(r.hold)
		<-done
		if n := len(r.reads()); n != 10 {
			t.Errorf("read %d rows, want 10", n)
		}
		if r.most > aheadWorkers {
			t.Errorf("up to %d reads ran at once, want at most %d", r.most, aheadWorkers)
		}
	})
}

func TestAheadResetCancels(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := newAhead(t, r, 0, 4, time.Millisecond)
		cmd := a.Window(rowsOf(5), 0)
		done := make(chan struct{})
		go func() {
			run(cmd)
			close(done)
		}()
		synctest.Wait()
		// Another repository.
		a.Reset(t.Context())
		<-done
		if got := r.reads(); len(got) != aheadWorkers {
			t.Errorf("read %v, want only the first %d before the reset", got, aheadWorkers)
		}
		if n := len(r.cancelled); n != aheadWorkers {
			t.Errorf("%d reads cancelled, want %d", n, aheadWorkers)
		}
	})
}

func TestAheadStopsAtRateLimit(t *testing.T) {
	r := newReader()
	r.err = &core.RateLimitError{Reset: time.Now()}
	a := newAhead(t, r, 0, 29, time.Millisecond)
	run(a.Window(rowsOf(30), 0))
	if n := len(r.reads()); n > aheadWorkers {
		t.Errorf("read %d rows after the rate limit, want at most the %d in flight", n, aheadWorkers)
	}

	// Nothing more is read, not even once the cursor moves.
	r.err = nil
	before := len(r.reads())
	a.Reset(t.Context())
	if cmd := a.Window(rowsOf(30), 0); cmd != nil {
		t.Error("the first window read ahead while rate limited")
	}
	if cmd := a.Window(rowsOf(30), 7); cmd != nil {
		t.Error("a move read ahead while rate limited")
	}

	a.Resume()
	a.Reset(t.Context())
	run(win(a, 0, 1).Window(rowsOf(2), 0))
	if n := len(r.reads()); n != before+2 {
		t.Errorf("read %d rows after Resume, want 2", n-before)
	}
}

// TestAheadWindowNoRowDropsRest checks that the cursor leaving the rows,
// such as when the list loses the focus, drops the rest still to come.
func TestAheadWindowNoRowDropsRest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		a := newAhead(t, r, 0, 0, 150*time.Millisecond)
		run(a.Window(rowsOf(30), 0))
		pending := a.Window(rowsOf(30), 5)
		if cmd := a.Window(nil, -1); cmd != nil {
			t.Error("no row started a rest")
		}
		if cmd := rest(t, a, pending); cmd != nil {
			t.Error("the rest of a row the cursor left read it")
		}
		if got, want := r.reads(), []int{1}; !slices.Equal(got, want) {
			t.Errorf("read %v, want only the first window, %v", got, want)
		}
	})
}

func TestNilAhead(t *testing.T) {
	var a *Ahead[int]
	a.Reset(t.Context())
	a.Resume()
	if a.Window(rowsOf(3), 0) != nil || a.On() || a.Rested(AheadMsg{}) != nil {
		t.Error("a nil Ahead read ahead")
	}
}

func TestAheadWindowFirstReadsAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.cached[3] = true
		a := newAhead(t, r, 1, 4, time.Hour)

		if cmd := a.Window(nil, 0); cmd != nil {
			t.Error("Window read ahead before the list loaded")
		}
		// The list loading counts as a rest: no delay, and nothing above the
		// first row.
		cmd := a.Window(rowsOf(30), 0)
		if cmd == nil {
			t.Fatal("the first window read nothing")
		}
		if _, ok := cmd().(AheadMsg); ok {
			t.Fatal("the first window waited for the cursor to rest")
		}
		if got, want := r.reads(), []int{1, 2, 4, 5}; !slices.Equal(got, want) {
			t.Errorf("read %v, want the row under the cursor and the four below it but the cached one, %v", got, want)
		}
		if cmd := a.Window(rowsOf(30), 0); cmd != nil {
			t.Error("the same window read again without the cursor moving")
		}
		// A reload brings a new first row, which is read once the cursor
		// rests, alone.
		reloaded := func(i int) (int, bool) { return []int{9, 1, 2, 3, 4}[i], i < 5 }
		cmd = a.Window(reloaded, 0)
		msg, ok := cmd().(AheadMsg)
		if !ok {
			t.Fatal("a reloaded window didn't wait for the cursor to rest")
		}
		run(a.Rested(msg))
		if got, want := r.reads(), []int{1, 2, 4, 5, 9}; !slices.Equal(got, want) {
			t.Errorf("read %v, want the new row too, %v", got, want)
		}
	})
}

func TestWindowRows(t *testing.T) {
	// Row 10 is at index 9. The row under the cursor comes first, then
	// the nearest rows, the one below before the one above.
	if got, want := windowRows(rowsOf(30), 9, 2, 3), []int{10, 11, 9, 12, 8, 13}; !slices.Equal(got, want) {
		t.Errorf("window %v, want %v", got, want)
	}
	// Nothing above the first row nor past the loaded ones.
	if got, want := windowRows(rowsOf(3), 0, 2, 4), []int{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("window %v, want %v", got, want)
	}
	if got, want := windowRows(rowsOf(30), 4, 0, 0), []int{5}; !slices.Equal(got, want) {
		t.Errorf("window %v, want the row under the cursor only, %v", got, want)
	}
}

func TestAheadWindowStartsNearestFirst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := newAhead(t, r, 0, 0, 0)
		done := make(chan struct{})
		go func() {
			run(win(a, 2, 3).Window(rowsOf(30), 9))
			close(done)
		}()
		synctest.Wait()
		// The farther rows wait for a worker.
		if got, want := r.reads(), []int{9, 10, 11}; !slices.Equal(got, want) {
			t.Errorf("started %v, want the row under the cursor and the nearest, %v", got, want)
		}
		close(r.hold)
		<-done
		if got, want := r.reads(), []int{8, 9, 10, 11, 12, 13}; !slices.Equal(got, want) {
			t.Errorf("read %v, want %v", got, want)
		}
	})
}

func TestAheadWindowRereadsOnEveryRest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		a := newAhead(t, r, 0, 0, 150*time.Millisecond)
		rows := rowsOf(30)
		run(win(a, 0, 1).Window(rows, 0))

		// The cursor passes row 5 for row 6 before it rests.
		passed := win(a, 0, 1).Window(rows, 4)
		rested := win(a, 0, 1).Window(rows, 5)
		start := time.Now()
		if cmd := rest(t, a, passed); cmd != nil {
			t.Error("the rest of a window the cursor left read it")
		}
		run(rest(t, a, rested))
		if waited := time.Since(start); waited != 150*time.Millisecond {
			t.Errorf("waited %v, want the delay", waited)
		}
		if got, want := r.reads(), []int{1, 2, 6, 7}; !slices.Equal(got, want) {
			t.Errorf("read %v, want the first window and the one rested on, %v", got, want)
		}

		// Back on the first row, which went stale meanwhile: the rest
		// reads the window again, and only what isn't cached.
		r.mu.Lock()
		delete(r.cached, 2)
		r.mu.Unlock()
		run(rest(t, a, win(a, 0, 1).Window(rows, 0)))
		if got, want := r.reads(), []int{1, 2, 2, 6, 7}; !slices.Equal(got, want) {
			t.Errorf("read %v, want the stale row again, %v", got, want)
		}
		if cmd := rest(t, a, win(a, 0, 1).Window(rows, 5)); cmd != nil {
			t.Error("a rest on a window all cached read it")
		}
	})
}

func TestAheadWindowSkipsRowsWithNoDetail(t *testing.T) {
	r := newReader()
	a := newAhead(t, r, 0, 0, 0)
	// Even rows, such as directories, have no detail, but count toward the
	// window.
	at := func(i int) (int, bool) { return i + 1, i%2 == 1 }
	run(win(a, 2, 2).Window(at, 4))
	if got, want := r.reads(), []int{4, 6}; !slices.Equal(got, want) {
		t.Errorf("read %v, want the rows with a detail within two of row 5, %v", got, want)
	}
}

func TestAheadWindowCancelsTheLastRest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := newAhead(t, r, 0, 0, time.Millisecond)
		first := win(a, 0, 1).Window(rowsOf(30), 10)
		done := make(chan struct{})
		go func() {
			run(first)
			close(done)
		}()
		synctest.Wait()
		// The cursor rests elsewhere, which cancels the reads of the last
		// rest.
		second := rest(t, a, win(a, 0, 0).Window(rowsOf(30), 20))
		<-done
		r.mu.Lock()
		cancelled := slices.Sorted(slices.Values(r.cancelled))
		r.mu.Unlock()
		if want := []int{11, 12}; !slices.Equal(cancelled, want) {
			t.Errorf("cancelled %v, want the reads of the last rest, %v", cancelled, want)
		}
		close(r.hold)
		run(second)
		if got := r.reads(); !slices.Contains(got, 21) {
			t.Errorf("read %v, want row 21 too", got)
		}
	})
}

func TestAheadWindowResetReadsAtOnce(t *testing.T) {
	r := newReader()
	a := newAhead(t, r, 0, 0, time.Hour)
	run(win(a, 0, 0).Window(rowsOf(30), 0))
	// Another list, such as another repository's, loads.
	a.Reset(t.Context())
	cmd := win(a, 0, 0).Window(func(i int) (int, bool) { return i + 100, i < 30 }, 0)
	if cmd == nil {
		t.Fatal("the first window of a new list read nothing")
	}
	if _, ok := cmd().(AheadMsg); ok {
		t.Error("the first window of a new list waited for the cursor to rest")
	}
	if got, want := r.reads(), []int{1, 100}; !slices.Equal(got, want) {
		t.Errorf("read %v, want %v", got, want)
	}
}

// rowCounts returns the prefetch counters of the rows in stats, which the
// tests read ahead as kind row.
func rowCounts(stats *obs.Stats) obs.PrefetchStats {
	for _, p := range stats.Summary().Prefetch {
		if p.Kind == "row" {
			return p
		}
	}
	return obs.PrefetchStats{Kind: "row"}
}

func TestAheadCountsReadsInFlightOnce(t *testing.T) {
	tests := []struct {
		name string
		// during runs while the reads of the first three rows are held,
		// and returns the commands it started.
		during func(t *testing.T, a *Ahead[int]) tea.Cmd
		// after runs once they ended.
		after func(a *Ahead[int])
		fail  bool
		want  obs.PrefetchStats
	}{{
		name: "a rest on a window in flight",
		during: func(t *testing.T, a *Ahead[int]) tea.Cmd {
			t.Helper()
			return rest(t, a, a.Window(rowsOf(3), 1))
		},
		want: obs.PrefetchStats{Sent: 3, Read: 3},
	}, {
		name:   "the same window",
		during: func(_ *testing.T, a *Ahead[int]) tea.Cmd { return a.Window(rowsOf(3), 0) },
		want:   obs.PrefetchStats{Sent: 3, Read: 3},
	}, {
		name:   "opened while read",
		during: func(_ *testing.T, a *Ahead[int]) tea.Cmd { a.Opened(2); return nil },
		want:   obs.PrefetchStats{Sent: 3, Read: 3, Opened: 1},
	}, {
		name:  "opened once read",
		after: func(a *Ahead[int]) { a.Opened(2); a.Opened(2) },
		want:  obs.PrefetchStats{Sent: 3, Read: 3, Opened: 1},
	}, {
		name:   "opened while read, which failed",
		during: func(_ *testing.T, a *Ahead[int]) tea.Cmd { a.Opened(2); return nil },
		fail:   true,
		want:   obs.PrefetchStats{Sent: 3, Failed: 3},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				_, stats := captureLog(t)
				r := newReader()
				r.hold = make(chan struct{})
				if tt.fail {
					r.err = errors.New("boom")
				}
				// The window at the second row holds the first three
				// too, so moving there leaves their reads alone.
				a := newAhead(t, r, 1, 2, time.Millisecond)
				first := a.Window(rowsOf(3), 0)
				done := make(chan struct{})
				go func() {
					run(first)
					close(done)
				}()
				synctest.Wait()
				if tt.during != nil {
					if cmd := tt.during(t, a); cmd != nil {
						t.Error("read ahead a row whose read is in flight")
					}
				}
				close(r.hold)
				<-done
				if tt.after != nil {
					tt.after(a)
				}
				got := rowCounts(stats)
				got.Kind, got.Useful = "", 0
				if got != tt.want {
					t.Errorf("counted %+v, want %+v", got, tt.want)
				}
				if n := len(r.reads()); n != 3 {
					t.Errorf("read %d rows, want 3", n)
				}
			})
		})
	}
}

// TestAheadRereadsRowBack checks that a row that left the window and comes
// back is read again at once, rather than left to the read that was
// cancelled when it left.
func TestAheadRereadsRowBack(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := newAhead(t, r, 0, 0, time.Millisecond)
		first := a.Window(rowsOf(30), 10)
		done := make(chan struct{})
		go func() {
			run(first)
			close(done)
		}()
		synctest.Wait()
		away := rest(t, a, a.Window(rowsOf(30), 20))
		back := rest(t, a, a.Window(rowsOf(30), 10))
		if back == nil {
			t.Fatal("the window skipped the row whose read it cancelled")
		}
		<-done
		close(r.hold)
		run(away)
		run(back)
		if got, want := r.reads(), []int{11, 11}; !slices.Equal(got, want) {
			t.Errorf("read %v, want %v", got, want)
		}
	})
}

func TestAheadPause(t *testing.T) {
	tests := []struct {
		name string
		// pause pauses a before the first rows are read ahead, and
		// returns what resumes it.
		pause func(a *Ahead[int]) (resume func())
		// held is how many reads start while paused.
		held int
	}{{
		name:  "none",
		pause: func(*Ahead[int]) func() { return func() {} },
		held:  3,
	}, {
		name:  "one",
		pause: func(a *Ahead[int]) func() { return a.Pause() },
	}, {
		name: "two, both resumed",
		pause: func(a *Ahead[int]) func() {
			r1, r2 := a.Pause(), a.Pause()
			return func() { r1(); r2() }
		},
	}, {
		name: "resumed twice, then paused again",
		pause: func(a *Ahead[int]) func() {
			r := a.Pause()
			r()
			r()
			return a.Pause()
		},
	}, {
		name: "with others and none, as a modal from another list",
		pause: func(a *Ahead[int]) func() {
			var none *Ahead[string]
			return PauseAll(a, nil, none, NewAhead[string]("other", nil, nil))
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := newReader()
				a := newAhead(t, r, 0, 2, time.Millisecond)
				resume := tt.pause(a)
				cmd := a.Window(rowsOf(3), 0)
				done := make(chan struct{})
				go func() {
					run(cmd)
					close(done)
				}()
				synctest.Wait()
				if n := len(r.reads()); n != tt.held {
					t.Errorf("%d reads started while paused, want %d", n, tt.held)
				}
				resume()
				<-done
				if n := len(r.reads()); n != 3 {
					t.Errorf("read %d rows once resumed, want 3", n)
				}
			})
		})
	}
}

func TestAheadPauseLetsReadsInFlightGoOn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		// The window one row down holds the first five rows too, so moving
		// there leaves their reads alone.
		a := newAhead(t, r, 1, 4, time.Millisecond)
		first := a.Window(rowsOf(30), 0)
		done := make(chan struct{})
		go func() {
			run(first)
			close(done)
		}()
		synctest.Wait()
		resume := a.Pause()
		// The read of the row that comes into the window waits too.
		next := rest(t, a, a.Window(rowsOf(30), 1))
		nexted := make(chan struct{})
		go func() {
			run(next)
			close(nexted)
		}()
		close(r.hold)
		synctest.Wait()
		if got, want := r.reads(), []int{1, 2, 3}; !slices.Equal(got, want) {
			t.Errorf("read %v while paused, want only those in flight before, %v", got, want)
		}
		resume()
		<-done
		<-nexted
		if got, want := r.reads(), []int{1, 2, 3, 4, 5, 6}; !slices.Equal(got, want) {
			t.Errorf("read %v once resumed, want %v", got, want)
		}
	})
}

func TestAheadResetCancelsPausedReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, stats := captureLog(t)
		r := newReader()
		a := newAhead(t, r, 0, 2, time.Millisecond)
		resume := a.Pause()
		defer resume()
		cmd := a.Window(rowsOf(3), 0)
		done := make(chan struct{})
		go func() {
			run(cmd)
			close(done)
		}()
		synctest.Wait()
		a.Reset(t.Context())
		<-done
		if n := len(r.reads()); n != 0 {
			t.Errorf("read %d rows, want none", n)
		}
		if p := rowCounts(stats); p.Sent != 0 || p.Canceled != 3 {
			t.Errorf("counted %+v, want 3 canceled and none sent", p)
		}
	})
}

func TestNilAheadPause(_ *testing.T) {
	var a *Ahead[int]
	resume := a.Pause()
	resume()
}

func TestAheadStopsOnBudget(t *testing.T) {
	_, stats := captureLog(t)
	obs.SetPrefetchBudget(10)
	t.Cleanup(func() { obs.SetPrefetchBudget(0) })
	r := newReader()
	// Each read costs a fifth of the budget GitHub leaves reads ahead
	// before it reported the limit.
	read := func(ctx context.Context, k int) error {
		obs.ChargeGraphQL(ctx, 100)
		return r.readRow(ctx, k)
	}
	a := aheadOf(t, read, r.current, 0, 29, time.Millisecond)
	run(a.Window(rowsOf(30), 0))
	if n := len(r.reads()); n < 5 || n > 5+aheadWorkers-1 {
		t.Errorf("read %d rows, want 5 and those already in flight", n)
	}
	if !stats.Summary().Budget.Spent {
		t.Error("the budget isn't spent")
	}

	// Nothing more is read for the session, even for another list.
	n := len(r.reads())
	a.Reset(t.Context())
	other := func(i int) (int, bool) { return 100 + i, true }
	if cmd := a.Window(other, 0); cmd != nil {
		t.Error("the first window read ahead over the budget")
	}
	// Moving on skips nothing new.
	for i := range 5 {
		if cmd := a.Window(other, 1+i); cmd != nil {
			t.Error("a move read ahead over the budget")
		}
	}
	if got := len(r.reads()); got != n {
		t.Errorf("read %d more rows over the budget", got-n)
	}
	// The rows of the first list not read, and the first call after.
	if p, want := rowCounts(stats), int64(30-n+1); p.OverBudget != want {
		t.Errorf("counted %d reads skipped for the budget, want %d", p.OverBudget, want)
	}
}

func TestAheadPausedReadSkipsWhatGotCached(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, stats := captureLog(t)
		r := newReader()
		a := newAhead(t, r, 0, 0, time.Millisecond)
		resume := a.Pause()
		read := a.Window(rowsOf(30), 8)
		done := make(chan struct{})
		go func() {
			run(read)
			close(done)
		}()
		synctest.Wait()
		// The detail opened reads the row, and then resumes.
		r.mu.Lock()
		r.cached[9] = true
		r.mu.Unlock()
		resume()
		<-done
		if got := r.reads(); len(got) != 0 {
			t.Errorf("read %v, want nothing once the row got cached", got)
		}
		if p := rowCounts(stats); p.Sent != 0 || p.Cached != 1 {
			t.Errorf("counted %+v, want 1 skipped as cached", p)
		}
	})
}

// Reads not sent are logged at debug level, with why.
func TestAheadLogsSkipped(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	r := newReader()
	r.err = &core.RateLimitError{Reset: time.Now()}
	a := newAhead(t, r, 0, 29, time.Millisecond)
	run(a.Window(rowsOf(30), 0))
	a.Window(rowsOf(30), 7)

	var limit int
	for line := range strings.Lines(buf.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		if m["msg"] == "prefetch skipped" {
			if m["level"] != "DEBUG" || m["kind"] != "row" || m["why"] != "limit" {
				t.Errorf("record = %v, want a debug record of rows skipped for the limit", m)
			}
			limit++
		}
	}
	if limit == 0 {
		t.Errorf("no prefetch skipped record:\n%s", buf.String())
	}
}

// TestAheadWindowKeepsReadsStillInIt checks that a rest stops only the
// reads of the rows that left the window, and neither stops nor sends
// again those of the rows still in it.
func TestAheadWindowKeepsReadsStillInIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := newAhead(t, r, 0, 0, time.Millisecond)
		win(a, 0, 2)
		first := a.Window(rowsOf(30), 10)
		done := make(chan struct{})
		go func() {
			run(first)
			close(done)
		}()
		synctest.Wait()
		// One row down: row 11 leaves the window, 12 and 13 stay, and 14
		// comes in.
		second := rest(t, a, a.Window(rowsOf(30), 11))
		go run(second)
		synctest.Wait()
		r.mu.Lock()
		cancelled, started := slices.Clone(r.cancelled), slices.Sorted(slices.Values(r.read))
		r.mu.Unlock()
		if want := []int{11}; !slices.Equal(cancelled, want) {
			t.Errorf("cancelled %v, want only the row that left, %v", cancelled, want)
		}
		if want := []int{11, 12, 13, 14}; !slices.Equal(started, want) {
			t.Errorf("started %v, want each row once, %v", started, want)
		}
		close(r.hold)
		<-done
		synctest.Wait()
	})
}

// TestAheadWindowAfterResume checks that a window the rate limit stopped
// is read once the reads resume, though the cursor didn't move.
func TestAheadWindowAfterResume(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.err = core.ErrRateLimited
		a := newAhead(t, r, 0, 0, time.Millisecond)
		win(a, 0, 1)
		run(a.Window(rowsOf(30), 0))
		if cmd := a.Window(rowsOf(30), 0); cmd != nil {
			t.Error("the same window waited again under the rate limit")
		}
		r.mu.Lock()
		r.err = nil
		n := len(r.read)
		r.mu.Unlock()
		a.Resume()
		run(rest(t, a, a.Window(rowsOf(30), 0)))
		if got := r.reads(); len(got) <= n {
			t.Errorf("read %v after Resume, want the window again", got)
		}
	})
}

func TestWindowRowsEdges(t *testing.T) {
	var asked []int
	at := func(i int) (int, bool) {
		asked = append(asked, i)
		// Rows 4 and 5 are the same item, as a list may show twice.
		if i == 5 {
			return 4, true
		}
		return i, i < 30
	}
	if got, want := windowRows(at, 4, 2, 2), []int{4, 3, 6, 2}; !slices.Equal(got, want) {
		t.Errorf("window %v, want each item once, %v", got, want)
	}
	asked = nil
	windowRows(at, 1, 3, 0)
	if slices.ContainsFunc(asked, func(i int) bool { return i < 0 }) {
		t.Errorf("asked for rows %v, want none above the first", asked)
	}
	if got := windowRows(at, -1, 1, 1); len(got) != 0 {
		t.Errorf("window %v for no row, want none", got)
	}
}

func TestAheadConfigureOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := newAhead(t, r, 0, 0, time.Millisecond)
		win(a, 0, 1)
		cmd := a.Window(rowsOf(30), 0)
		done := make(chan struct{})
		go func() {
			run(cmd)
			close(done)
		}()
		synctest.Wait()
		a.Configure(config.Resolved{Enabled: false})
		<-done
		if a.On() || a.Window(rowsOf(30), 5) != nil {
			t.Error("the reads went on once the settings turned them off")
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if len(r.cancelled) != 2 {
			t.Errorf("cancelled %v, want the reads in flight", r.cancelled)
		}
	})
}

// TestAheadsShareSlots checks that two Aheads that share slots keep to
// them together.
func TestAheadsShareSlots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		slots := NewSlots(2)
		cmds := make([]tea.Cmd, 0, 2)
		for range 2 {
			a := newAhead(t, r, 0, 0, 0)
			a.Share(slots)
			cmds = append(cmds, win(a, 0, 3).Window(rowsOf(30), len(cmds)*10))
		}
		done := make(chan struct{})
		go func() {
			run(tea.Batch(cmds...))
			close(done)
		}()
		synctest.Wait()
		r.mu.Lock()
		if r.inFlight != 2 {
			t.Errorf("%d reads in flight, want the 2 slots", r.inFlight)
		}
		r.mu.Unlock()
		slots.SetSize(3)
		synctest.Wait()
		r.mu.Lock()
		if r.inFlight != 3 {
			t.Errorf("%d reads in flight after the slots grew, want 3", r.inFlight)
		}
		r.mu.Unlock()
		close(r.hold)
		<-done
	})
}

// TestAheadWindowFirstShownOnly checks that only the window of the list as
// it first shows is read at once: when its first rows have nothing to
// read, the first row that does waits for the cursor to rest.
func TestAheadWindowFirstShownOnly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		a := newAhead(t, r, 0, 0, time.Hour)
		// The first five rows are folders, with nothing to read.
		at := func(i int) (int, bool) { return i + 1, i >= 5 && i < 30 }
		if cmd := win(a, 0, 0).Window(at, 0); cmd != nil {
			run(cmd)
		}
		cmd := a.Window(at, 5)
		if cmd == nil {
			t.Fatal("the first file started no rest")
		}
		start := time.Now()
		if _, ok := cmd().(AheadMsg); !ok || time.Since(start) != time.Hour {
			t.Error("the first file was read at once, want it to wait for the rest")
		}
		if got := r.reads(); len(got) != 0 {
			t.Errorf("read %v before the cursor rested", got)
		}
	})
}

// TestAheadSkipsRowsThatFailed checks that a row whose read ahead failed
// isn't read ahead again for a while, however often the cursor rests near
// it, and is once the while has passed.
func TestAheadSkipsRowsThatFailed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, stats := captureLog(t)
		r := newReader()
		r.err = errors.New("boom")
		a := newAhead(t, r, 0, 1, time.Millisecond)
		rows := rowsOf(30)
		run(a.Window(rows, 0))
		// One row down: only the row that came in is sent.
		run(rest(t, a, a.Window(rows, 1)))
		if got, want := r.reads(), []int{1, 2, 3}; !slices.Equal(got, want) {
			t.Errorf("read %v, want each row once, %v", got, want)
		}
		if cmd := rest(t, a, a.Window(rows, 0)); cmd != nil {
			t.Error("a rest on rows that failed lately read them again")
		}
		if p := rowCounts(stats); p.Sent != 3 || p.Failed != 3 {
			t.Errorf("counted %+v, want 3 sent and failed", p)
		}

		time.Sleep(aheadFailedFor)
		r.mu.Lock()
		r.err = nil
		r.mu.Unlock()
		run(rest(t, a, a.Window(rows, 1)))
		if got, want := r.reads(), []int{1, 2, 2, 3, 3}; !slices.Equal(got, want) {
			t.Errorf("read %v, want the rows again once the while passed, %v", got, want)
		}
	})
}

// TestAheadResetForgetsFailures checks that a new list reads ahead the rows
// whose reads failed for the last one.
func TestAheadResetForgetsFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.err = errors.New("boom")
		a := newAhead(t, r, 0, 0, time.Millisecond)
		run(a.Window(rowsOf(30), 0))
		r.mu.Lock()
		r.err = nil
		r.mu.Unlock()
		a.Reset(t.Context())
		run(a.Window(rowsOf(30), 0))
		if got, want := r.reads(), []int{1, 1}; !slices.Equal(got, want) {
			t.Errorf("read %v, want the row again for the new list, %v", got, want)
		}
	})
}

// TestAheadRereadsRowsNotFailed checks that rows whose reads got no answer
// from GitHub, only a server error, or the rate limit, or were cancelled,
// aren't held back as failed.
func TestAheadRereadsRowsNotFailed(t *testing.T) {
	for name, err := range map[string]error{
		"offline":      fmt.Errorf("list: %w", core.ErrOffline),
		"server error": fmt.Errorf("list: %w", core.ErrUnavailable),
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := newReader()
				r.err = err
				a := newAhead(t, r, 0, 0, time.Millisecond)
				run(a.Window(rowsOf(30), 0))
				r.mu.Lock()
				r.err = nil
				r.mu.Unlock()
				// GitHub answers again: away and back, the row is read again.
				run(rest(t, a, a.Window(rowsOf(30), 1)))
				run(rest(t, a, a.Window(rowsOf(30), 0)))
				if got, want := r.reads(), []int{1, 1, 2}; !slices.Equal(got, want) {
					t.Errorf("read %v, want row 1 again, %v", got, want)
				}
			})
		})
	}
	t.Run("rate limit", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := newReader()
			r.err = core.ErrRateLimited
			a := newAhead(t, r, 0, 0, time.Millisecond)
			run(a.Window(rowsOf(30), 0))
			if a.failed.recent(1, time.Now()) {
				t.Error("a read the rate limit stopped counts as failed")
			}
			r.mu.Lock()
			r.err = nil
			r.mu.Unlock()
			a.Resume()
			run(rest(t, a, a.Window(rowsOf(30), 0)))
			if got, want := r.reads(), []int{1, 1}; !slices.Equal(got, want) {
				t.Errorf("read %v, want the row again once resumed, %v", got, want)
			}
		})
	})
	t.Run("cancelled", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			r := newReader()
			r.hold = make(chan struct{})
			a := newAhead(t, r, 0, 0, time.Millisecond)
			first := a.Window(rowsOf(30), 10)
			done := make(chan struct{})
			go func() {
				run(first)
				close(done)
			}()
			synctest.Wait()
			// Moving away cancels the read of row 11, which unwinds.
			away := rest(t, a, a.Window(rowsOf(30), 20))
			<-done
			close(r.hold)
			run(away)
			run(rest(t, a, a.Window(rowsOf(30), 10)))
			if got, want := r.reads(), []int{11, 11, 21}; !slices.Equal(got, want) {
				t.Errorf("read %v, want row 11 again once back, %v", got, want)
			}
		})
	})
}

// TestAheadResumeRetriesFailures checks that Resume, as a refresh calls it,
// tries again at once the rows whose reads failed lately.
func TestAheadResumeRetriesFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.err = errors.New("boom")
		a := newAhead(t, r, 0, 0, time.Millisecond)
		run(a.Window(rowsOf(30), 0))
		r.mu.Lock()
		r.err = nil
		r.mu.Unlock()
		if cmd := rest(t, a, a.Window(rowsOf(30), 1)); cmd != nil {
			run(cmd)
		}
		if cmd := rest(t, a, a.Window(rowsOf(30), 0)); cmd != nil {
			t.Error("a rest read again a row that failed lately")
		}
		a.Resume()
		run(rest(t, a, a.Window(rowsOf(30), 0)))
		if got, want := r.reads(), []int{1, 1, 2}; !slices.Equal(got, want) {
			t.Errorf("read %v, want row 1 again once resumed, %v", got, want)
		}
	})
}
