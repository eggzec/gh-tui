package ui

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
)

// pullsKinds returns the kinds details and comments of prefetch.pulls,
// read through d and c.
func pullsKinds(d, c *reader) []AheadKind[int] {
	return []AheadKind[int]{
		{Name: "details", Log: "row", Read: d.readRow, Current: d.current},
		{Name: "comments", Log: "row_comments", Read: c.readRow, Current: c.current},
	}
}

func TestAheadsKindsApart(t *testing.T) {
	d, c := newReader(), newReader()
	a := NewAheads(t.Context(), "pulls", pullsKinds(d, c)...)
	if a.On() || a.Window(rowsOf(30), 0) != nil {
		t.Fatal("Aheads read ahead before it had the settings")
	}

	p := config.Default().Prefetch
	p.Window = config.Window{Before: 0, After: 2}
	p.Pulls.Comments.Enabled = new(false)
	a.Configure(p)
	run(a.Window(rowsOf(30), 0))
	if got, want := d.reads(), []int{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("details read %v, want %v", got, want)
	}
	if got := c.reads(); len(got) != 0 {
		t.Errorf("comments read %v, want none while they are off", got)
	}

	// Comments turn on with a window of their own, from the next move.
	p.Pulls.Comments.Enabled = new(true)
	p.Pulls.Comments.Window.After = new(0)
	p.Pulls.Comments.Rest = new(time.Duration(0))
	a.Configure(p)
	if !a.On() {
		t.Error("On = false with both kinds on")
	}
	run(a.Window(rowsOf(30), 0))
	if got, want := c.reads(), []int{1}; !slices.Equal(got, want) {
		t.Errorf("comments read %v, want the row under the cursor, %v", got, want)
	}
}

func TestAheadsUnknownKindReadsNothing(t *testing.T) {
	r := newReader()
	a := NewAheads(t.Context(), "pulls", AheadKind[int]{Name: "nope", Log: "row", Read: r.readRow, Current: r.current})
	a.Configure(config.Default().Prefetch)
	if a.On() || a.Window(rowsOf(30), 0) != nil {
		t.Error("a kind the settings don't have read ahead")
	}
}

func TestNilAheads(t *testing.T) {
	var a *Aheads[int]
	a.Configure(config.Default().Prefetch)
	a.Reset(t.Context())
	a.Resume()
	a.Opened(1)
	a.Pause()()
	if a.On() || a.Window(rowsOf(3), 0) != nil || a.Rested(AheadMsg{}) != nil {
		t.Error("a nil Aheads read ahead")
	}
}

// runAtOnce runs cmd, and each command of a batch it returns in a
// goroutine of its own, as Bubble Tea does, to the end.
func runAtOnce(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	b, ok := cmd().(tea.BatchMsg)
	if !ok {
		return
	}
	var wg sync.WaitGroup
	for _, c := range b {
		wg.Go(func() { runAtOnce(c) })
	}
	wg.Wait()
}

// counted wraps the reads of readers so that they count the reads in
// flight together, and the most at once.
type counted struct {
	mu             sync.Mutex
	inFlight, most int
}

func (c *counted) wrap(r *reader) func(ctx context.Context, k int) error {
	return func(ctx context.Context, k int) error {
		c.mu.Lock()
		c.inFlight++
		c.most = max(c.most, c.inFlight)
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			c.inFlight--
			c.mu.Unlock()
		}()
		return r.readRow(ctx, k)
	}
}

func (c *counted) counts() (inFlight, most int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inFlight, c.most
}

// sharedPages returns the reads ahead of the pulls and issues pages, with
// every kind on and a window of four rows, that share slots, and the
// readers of their kinds, which hold their reads until hold is closed.
func sharedPages(t *testing.T, slots *Slots, hold chan struct{}, c *counted) (pulls, issues *Aheads[int], rs []*reader) {
	t.Helper()
	p := config.Default().Prefetch
	p.Window, p.Rest = config.Window{Before: 0, After: 3}, 0
	page := func(name string) *Aheads[int] {
		kinds := make([]AheadKind[int], 0, 2)
		for _, kind := range []string{"details", "comments"} {
			r := newReader()
			r.hold = hold
			rs = append(rs, r)
			kinds = append(kinds, AheadKind[int]{Name: kind, Log: name + "_" + kind, Read: c.wrap(r), Current: r.current})
		}
		a := NewAheads(t.Context(), name, kinds...)
		a.Share(slots)
		a.Configure(p)
		return a
	}
	return page("pulls"), page("issues"), rs
}

// TestAheadsPagesKeepToSharedSlots checks that the reads ahead of two
// pages, each with two kinds, keep together to the slots they share, and
// that they all read in the end.
func TestAheadsPagesKeepToSharedSlots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const limit = 2
		var c counted
		hold := make(chan struct{})
		pulls, issues, rs := sharedPages(t, NewSlots(limit), hold, &c)
		done := make(chan struct{})
		go func() {
			runAtOnce(tea.Batch(pulls.Window(rowsOf(30), 0), issues.Window(rowsOf(30), 10)))
			close(done)
		}()
		synctest.Wait()
		if n, _ := c.counts(); n != limit {
			t.Errorf("%d reads in flight, want the %d slots", n, limit)
		}
		close(hold)
		<-done
		if _, most := c.counts(); most > limit {
			t.Errorf("up to %d reads ran at once, want at most %d", most, limit)
		}
		for _, r := range rs {
			if got := len(r.reads()); got != 4 {
				t.Errorf("a kind read %d rows, want the 4 of its window", got)
			}
		}
	})
}

// TestAheadsPausedPageHoldsNoSlot checks that the reads of a paused page
// wait without holding the slots, so that another page that shares them
// reads meanwhile.
func TestAheadsPausedPageHoldsNoSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var c counted
		pulls, issues, rs := sharedPages(t, NewSlots(1), nil, &c)
		resume := pulls.Pause()
		paused := make(chan struct{})
		go func() {
			run(pulls.Window(rowsOf(30), 0))
			close(paused)
		}()
		synctest.Wait()
		// rs holds the readers of pulls, then those of issues.
		run(issues.Window(rowsOf(30), 10))
		for _, r := range rs[2:] {
			if got := len(r.reads()); got != 4 {
				t.Errorf("an issues kind read %d rows while pulls paused, want 4", got)
			}
		}
		for _, r := range rs[:2] {
			if got := r.reads(); len(got) != 0 {
				t.Errorf("a pulls kind read %v while paused", got)
			}
		}
		resume()
		<-paused
		for _, r := range rs[:2] {
			if got := len(r.reads()); got != 4 {
				t.Errorf("a pulls kind read %d rows once resumed, want 4", got)
			}
		}
	})
}
