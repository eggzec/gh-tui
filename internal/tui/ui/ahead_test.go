package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestAheadFirstRows(t *testing.T) {
	r := newReader()
	r.cached[2] = true
	a := NewAhead("row", r.readRow, r.current, 5, time.Millisecond)
	a.Reset(t.Context())

	if cmd := a.First(rowsOf(0)); cmd != nil {
		t.Error("First read ahead before the list loaded")
	}
	run(a.First(rowsOf(30)))
	if got, want := r.reads(), []int{1, 3, 4, 5}; !slices.Equal(got, want) {
		t.Errorf("read %v, want the first five rows but the cached one, %v", got, want)
	}
	if cmd := a.First(rowsOf(30)); cmd != nil {
		t.Error("First read the same rows again")
	}
	// A reload brings a new first row.
	run(a.First(func(i int) (int, bool) { return []int{9, 1, 2, 3, 4}[i], true }))
	if got, want := r.reads(), []int{1, 3, 4, 5, 9}; !slices.Equal(got, want) {
		t.Errorf("read %v, want the new row too, %v", got, want)
	}
}

func TestAheadBoundsReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := NewAhead("row", r.readRow, r.current, 10, time.Millisecond)
		a.Reset(t.Context())
		done := make(chan struct{})
		go func() {
			run(a.First(rowsOf(10)))
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
		a := NewAhead("row", r.readRow, r.current, 5, time.Millisecond)
		a.Reset(t.Context())
		done := make(chan struct{})
		go func() {
			run(a.First(rowsOf(5)))
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
	a := NewAhead("row", r.readRow, r.current, 30, time.Millisecond)
	a.Reset(t.Context())
	run(a.First(rowsOf(30)))
	if n := len(r.reads()); n > aheadWorkers {
		t.Errorf("read %d rows after the rate limit, want at most the %d in flight", n, aheadWorkers)
	}

	// Nothing more is read, not even under the cursor.
	r.err = nil
	before := len(r.reads())
	a.Reset(t.Context())
	if cmd := a.First(rowsOf(30)); cmd != nil {
		t.Error("First read ahead while rate limited")
	}
	if cmd := a.Moved(7, true); cmd != nil {
		t.Error("Moved read ahead while rate limited")
	}

	a.Resume()
	a.Reset(t.Context())
	run(a.First(rowsOf(2)))
	if n := len(r.reads()); n != before+2 {
		t.Errorf("read %d rows after Resume, want 2", n-before)
	}
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

func TestAheadHover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		a := NewAhead("row", r.readRow, r.current, 0, 150*time.Millisecond)
		a.Reset(t.Context())

		// The cursor passes row 1 for row 2 before the delay.
		passed := a.Moved(1, true)
		rested := a.Moved(2, true)
		start := time.Now()
		if cmd := rest(t, a, passed); cmd != nil {
			t.Error("the delay of a row the cursor left read it")
		}
		run(rest(t, a, rested))
		if waited := time.Since(start); waited != 150*time.Millisecond {
			t.Errorf("waited %v, want the delay", waited)
		}
		if got := r.reads(); !slices.Equal(got, []int{2}) {
			t.Errorf("read %v, want the row rested on", got)
		}
		if cmd := a.Moved(2, true); cmd != nil {
			t.Error("staying on a row started another delay")
		}
		// A cached row isn't read.
		if cmd := a.Moved(1, false); cmd != nil {
			t.Error("no row started a delay")
		}
		if cmd := a.Moved(2, true); cmd != nil {
			t.Error("a cached row started a delay")
		}
	})
}

func TestAheadHoverCancelsOlder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := NewAhead("row", r.readRow, r.current, 0, time.Millisecond)
		a.Reset(t.Context())

		first := rest(t, a, a.Moved(1, true))
		done := make(chan struct{})
		go func() {
			run(first)
			close(done)
		}()
		synctest.Wait()
		second := rest(t, a, a.Moved(2, true))
		<-done
		if !slices.Equal(r.cancelled, []int{1}) {
			t.Errorf("cancelled %v, want the older read", r.cancelled)
		}
		close(r.hold)
		run(second)
		if got := r.reads(); !slices.Equal(got, []int{1, 2}) {
			t.Errorf("read %v, want both rows", got)
		}
	})
}

func TestNilAhead(t *testing.T) {
	var a *Ahead[int]
	a.Reset(t.Context())
	a.Resume()
	if a.First(rowsOf(3)) != nil || a.Moved(1, true) != nil || a.Window(rowsOf(3), 0) != nil || a.On() || a.Rested(AheadMsg{}) != nil {
		t.Error("a nil Ahead read ahead")
	}
}

func TestAheadAround(t *testing.T) {
	r := newReader()
	r.cached[6] = true
	a := NewAhead("row", r.readRow, r.current, 0, 0)
	a.Reset(t.Context())

	if cmd := a.Around(rowsOf(30), 4, 0); cmd != nil {
		t.Error("Around read rows with none asked for")
	}
	// Row 5 is at index 4. Rows before the first and past the loaded ones
	// are left out, and so is the cached row.
	run(a.Around(rowsOf(7), 4, 3))
	if got, want := r.reads(), []int{2, 3, 4, 7}; !slices.Equal(got, want) {
		t.Errorf("read %v, want the rows around 5 but the cached one, %v", got, want)
	}
	if cmd := a.Around(rowsOf(7), 4, 3); cmd != nil {
		t.Error("Around read rows that are cached now")
	}
	run(a.Around(rowsOf(30), 0, 2))
	if got, want := r.reads(), []int{2, 3, 4, 7}; !slices.Equal(got, want) {
		t.Errorf("read %v, want nothing new, %v", got, want)
	}
}

func TestAheadAroundOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := NewAhead("row", r.readRow, r.current, 0, 0)
		a.Reset(t.Context())
		done := make(chan struct{})
		go func() {
			run(a.Around(rowsOf(30), 9, 2))
			close(done)
		}()
		synctest.Wait()
		// Row 10 is at index 9. The nearest rows start first, the one
		// after before the one before, and the farthest waits for a
		// worker.
		if got, want := r.reads(), []int{9, 11, 12}; !slices.Equal(got, want) {
			t.Errorf("started %v, want %v", got, want)
		}
		close(r.hold)
		<-done
		if got, want := r.reads(), []int{8, 9, 11, 12}; !slices.Equal(got, want) {
			t.Errorf("read %v, want %v", got, want)
		}
	})
}

func TestAheadAroundCancelsTheLastReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := NewAhead("row", r.readRow, r.current, 0, 0)
		a.Reset(t.Context())
		first := a.Around(rowsOf(30), 10, 1)
		done := make(chan struct{})
		go func() {
			run(first)
			close(done)
		}()
		synctest.Wait()
		// The cursor rests elsewhere, which cancels the reads around the
		// last place.
		_ = a.Around(rowsOf(30), 20, 1)
		<-done
		r.mu.Lock()
		cancelled := slices.Sorted(slices.Values(r.cancelled))
		r.mu.Unlock()
		if want := []int{10, 12}; !slices.Equal(cancelled, want) {
			t.Errorf("cancelled %v, want the reads around row 11, %v", cancelled, want)
		}
		close(r.hold)
	})
}

func TestAheadWindowFirstReadsAtOnce(t *testing.T) {
	r := newReader()
	r.cached[3] = true
	a := NewAhead("row", r.readRow, r.current, 0, time.Hour)
	a.Reset(t.Context())

	if cmd := win(a, 1, 4).Window(nil, 0); cmd != nil {
		t.Error("Window read ahead before the list loaded")
	}
	// The list loading counts as a rest: no delay, and nothing above the
	// first row.
	cmd := win(a, 1, 4).Window(rowsOf(30), 0)
	if cmd == nil {
		t.Fatal("the first window read nothing")
	}
	if _, ok := cmd().(AheadMsg); ok {
		t.Fatal("the first window waited for the cursor to rest")
	}
	if got, want := r.reads(), []int{1, 2, 4, 5}; !slices.Equal(got, want) {
		t.Errorf("read %v, want the row under the cursor and the four below it but the cached one, %v", got, want)
	}
	if cmd := win(a, 1, 4).Window(rowsOf(30), 0); cmd != nil {
		t.Error("the same window read again without the cursor moving")
	}
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
		a := NewAhead("row", r.readRow, r.current, 0, 0)
		a.Reset(t.Context())
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
		a := NewAhead("row", r.readRow, r.current, 0, 150*time.Millisecond)
		a.Reset(t.Context())
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
	a := NewAhead("row", r.readRow, r.current, 0, 0)
	a.Reset(t.Context())
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
		a := NewAhead("row", r.readRow, r.current, 0, time.Millisecond)
		a.Reset(t.Context())
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
	a := NewAhead("row", r.readRow, r.current, 0, time.Hour)
	a.Reset(t.Context())
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
		during func(a *Ahead[int]) tea.Cmd
		// after runs once they ended.
		after func(a *Ahead[int])
		fail  bool
		want  obs.PrefetchStats
	}{{
		name:   "hover on a first row",
		during: func(a *Ahead[int]) tea.Cmd { return a.Moved(1, true) },
		want:   obs.PrefetchStats{Sent: 3, Read: 3},
	}, {
		name:   "around a first row",
		during: func(a *Ahead[int]) tea.Cmd { return a.Around(rowsOf(3), 1, 1) },
		want:   obs.PrefetchStats{Sent: 3, Read: 3},
	}, {
		name:   "the same first rows",
		during: func(a *Ahead[int]) tea.Cmd { return a.First(rowsOf(3)) },
		want:   obs.PrefetchStats{Sent: 3, Read: 3},
	}, {
		name:   "opened while read",
		during: func(a *Ahead[int]) tea.Cmd { a.Opened(2); return nil },
		want:   obs.PrefetchStats{Sent: 3, Read: 3, Opened: 1},
	}, {
		name:  "opened once read",
		after: func(a *Ahead[int]) { a.Opened(2); a.Opened(2) },
		want:  obs.PrefetchStats{Sent: 3, Read: 3, Opened: 1},
	}, {
		name:   "opened while read, which failed",
		during: func(a *Ahead[int]) tea.Cmd { a.Opened(2); return nil },
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
				a := NewAhead("row", r.readRow, r.current, 3, time.Millisecond)
				a.Reset(t.Context())
				first := a.First(rowsOf(3))
				done := make(chan struct{})
				go func() {
					run(first)
					close(done)
				}()
				synctest.Wait()
				if tt.during != nil {
					if cmd := tt.during(a); cmd != nil {
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

func TestAheadRereadsAfterStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := newReader()
		r.hold = make(chan struct{})
		a := NewAhead("row", r.readRow, r.current, 0, 0)
		a.Reset(t.Context())
		first := a.Around(rowsOf(30), 10, 1)
		done := make(chan struct{})
		go func() {
			run(first)
			close(done)
		}()
		synctest.Wait()
		// The cursor rests there again, which cancels the reads and
		// starts them over, rather than leaving the rows to the cancelled
		// reads.
		again := a.Around(rowsOf(30), 10, 1)
		if again == nil {
			t.Fatal("Around skipped the rows of the reads it cancelled")
		}
		<-done
		close(r.hold)
		run(again)
		if got, want := r.reads(), []int{10, 10, 12, 12}; !slices.Equal(got, want) {
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
			return PauseAll(a, nil, none, NewAhead[string]("other", nil, nil, 0, 0))
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := newReader()
				a := NewAhead("row", r.readRow, r.current, 3, time.Millisecond)
				a.Reset(t.Context())
				resume := tt.pause(a)
				done := make(chan struct{})
				go func() {
					run(a.First(rowsOf(3)))
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
		a := NewAhead("row", r.readRow, r.current, 5, time.Millisecond)
		a.Reset(t.Context())
		done := make(chan struct{})
		go func() {
			run(a.First(rowsOf(5)))
			close(done)
		}()
		synctest.Wait()
		resume := a.Pause()
		// A hover read waits too.
		hover := rest(t, a, a.Moved(9, true))
		hovered := make(chan struct{})
		go func() {
			run(hover)
			close(hovered)
		}()
		close(r.hold)
		synctest.Wait()
		if got, want := r.reads(), []int{1, 2, 3}; !slices.Equal(got, want) {
			t.Errorf("read %v while paused, want only those in flight before, %v", got, want)
		}
		resume()
		<-done
		<-hovered
		if got, want := r.reads(), []int{1, 2, 3, 4, 5, 9}; !slices.Equal(got, want) {
			t.Errorf("read %v once resumed, want %v", got, want)
		}
	})
}

func TestAheadResetCancelsPausedReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, stats := captureLog(t)
		r := newReader()
		a := NewAhead("row", r.readRow, r.current, 3, time.Millisecond)
		a.Reset(t.Context())
		resume := a.Pause()
		defer resume()
		done := make(chan struct{})
		go func() {
			run(a.First(rowsOf(3)))
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
	a := NewAhead("row", read, r.current, 30, time.Millisecond)
	a.Reset(t.Context())
	run(a.First(rowsOf(30)))
	if n := len(r.reads()); n < 5 || n > 5+aheadWorkers-1 {
		t.Errorf("read %d rows, want 5 and those already in flight", n)
	}
	if !stats.Summary().Budget.Spent {
		t.Error("the budget isn't spent")
	}

	// Nothing more is read for the session, even for another list.
	n := len(r.reads())
	a.Reset(t.Context())
	if cmd := a.First(func(i int) (int, bool) { return 100 + i, true }); cmd != nil {
		t.Error("First read ahead over the budget")
	}
	if cmd := a.Moved(200, true); cmd != nil {
		t.Error("Moved read ahead over the budget")
	}
	if cmd := a.Around(rowsOf(300), 250, 2); cmd != nil {
		t.Error("Around read ahead over the budget")
	}
	if got := len(r.reads()); got != n {
		t.Errorf("read %d more rows over the budget", got-n)
	}
	// Moving on skips nothing new.
	for i := range 5 {
		if cmd := a.Moved(201+i, true); cmd != nil {
			t.Error("Moved read ahead over the budget")
		}
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
		a := NewAhead("row", r.readRow, r.current, 0, time.Millisecond)
		a.Reset(t.Context())
		resume := a.Pause()
		read := rest(t, a, a.Moved(9, true))
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

func TestAheadSet(t *testing.T) {
	a := NewAhead("pull", func(context.Context, int) error { return nil }, func(int) bool { return false }, 3, time.Second)
	a.Set(5, 2*time.Second)
	if a.rows != 5 || a.delay != 2*time.Second {
		t.Errorf("rows %d, delay %v, want 5 and 2s", a.rows, a.delay)
	}
	a.Set(-1, -time.Second)
	if a.rows != 0 || a.delay != 0 {
		t.Errorf("rows %d, delay %v, want none", a.rows, a.delay)
	}
	var none *Ahead[int]
	none.Set(1, time.Second)
}

// Reads not sent are logged at debug level, with why.
func TestAheadLogsSkipped(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	r := newReader()
	r.err = &core.RateLimitError{Reset: time.Now()}
	a := NewAhead("row", r.readRow, r.current, 30, time.Millisecond)
	a.Reset(t.Context())
	run(a.First(rowsOf(30)))
	a.Moved(7, true)

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
		a := NewAhead("row", r.readRow, r.current, 0, time.Millisecond)
		a.Reset(t.Context())
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
		a := NewAhead("row", r.readRow, r.current, 0, time.Millisecond)
		a.Reset(t.Context())
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
		a := NewAhead("row", r.readRow, r.current, 0, time.Millisecond)
		a.Reset(t.Context())
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
			a := NewAhead("row", r.readRow, r.current, 0, 0)
			a.Share(slots)
			a.Reset(t.Context())
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
		a := NewAhead("row", r.readRow, r.current, 0, time.Hour)
		a.Reset(t.Context())
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
