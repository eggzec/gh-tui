package ui

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
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
	a := NewAhead(r.readRow, r.current, 5, time.Millisecond)
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
		a := NewAhead(r.readRow, r.current, 10, time.Millisecond)
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
		a := NewAhead(r.readRow, r.current, 5, time.Millisecond)
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
	a := NewAhead(r.readRow, r.current, 30, time.Millisecond)
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
		a := NewAhead(r.readRow, r.current, 0, 150*time.Millisecond)
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
		a := NewAhead(r.readRow, r.current, 0, time.Millisecond)
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
	if a.First(rowsOf(3)) != nil || a.Moved(1, true) != nil || a.Rested(AheadMsg{}) != nil {
		t.Error("a nil Ahead read ahead")
	}
}
