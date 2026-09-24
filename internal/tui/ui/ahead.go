package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// aheadWorkers is the most rows whose details are read ahead at once, on
// top of the row under the cursor. A detail may take a few requests.
const aheadWorkers = 3

// Ahead reads the details of the rows of a list before they are opened, into
// the cache the detail reads from, so that they open at once: those of the
// first rows once the list loads, and that of the row the cursor rests on.
// K identifies a row, such as the query of its first comments. Create it
// with [NewAhead]; a nil *Ahead reads nothing.
//
// Each read costs requests, so few run at once, and the reads stop once
// GitHub reports the rate limit, until [Ahead.Resume].
type Ahead[K comparable] struct {
	id int64
	// seen remembers what was read, so that opening it counts as a use,
	// under the kind of detail, such as pull.
	seen  *obs.Prefetched[K]
	rows  int
	delay time.Duration
	read  func(ctx context.Context, k K) error
	// current reports whether the detail of k is cached already. It must
	// not do I/O.
	current func(k K) bool

	// ctx bounds the reads of the list shown, and cancel ends them.
	ctx    context.Context
	cancel context.CancelFunc
	// limited is shared with the reads in flight, which set it.
	limited *atomic.Bool
	// first are the first rows read ahead for the list.
	first []K

	// hovered is the row the cursor was last seen on, and seq counts the
	// times it moved, so that only the latest delay fires.
	hovered    K
	hasHovered bool
	seq        int
	stopHover  context.CancelFunc
	// stopAround cancels the reads around the cursor still in flight.
	stopAround context.CancelFunc
}

// AheadMsg reports that the cursor rested on a row. Sections pass it to
// [Ahead.Rested].
type AheadMsg struct {
	id  int64
	seq int
}

var lastAhead atomic.Int64

// NewAhead returns an Ahead that reads the detail of a row with read and
// asks current whether it is cached already. It reads the first rows of a
// list, and the row under the cursor once it has rested there for delay.
// Kind names the detail in the log, such as pull.
func NewAhead[K comparable](kind string, read func(ctx context.Context, k K) error, current func(k K) bool, rows int, delay time.Duration) *Ahead[K] {
	return &Ahead[K]{
		id:      lastAhead.Add(1),
		seen:    obs.NewPrefetched[K](kind),
		rows:    max(rows, 0),
		delay:   max(delay, 0),
		read:    read,
		current: current,
		ctx:     context.Background(),
		cancel:  func() {},
		limited: new(atomic.Bool),
	}
}

// Reset cancels the reads of the list shown, for a new list whose reads
// parent bounds.
func (a *Ahead[K]) Reset(parent context.Context) {
	if a == nil {
		return
	}
	a.cancel()
	a.ctx, a.cancel = context.WithCancel(parent)
	a.first = a.first[:0]
	var zero K
	a.hovered, a.hasHovered = zero, false
	a.seq++
	a.stopHover, a.stopAround = nil, nil
}

// Opened records that the detail of k was opened, so that the summary
// counts it as a use if it was read ahead.
func (a *Ahead[K]) Opened(k K) {
	if a != nil {
		a.seen.Opened(k)
	}
}

// Resume reads ahead again after GitHub reported the rate limit, such as
// for another repository.
func (a *Ahead[K]) Resume() {
	if a != nil {
		a.limited.Store(false)
	}
}

// First reads the details of the first rows of the list, which at returns
// by index, and false for a row not loaded. It reads them again only when
// the first rows change, and skips those cached.
func (a *Ahead[K]) First(at func(i int) (K, bool)) tea.Cmd {
	if a == nil || a.rows == 0 || a.limited.Load() || a.same(at) {
		return nil
	}
	a.first = a.first[:0]
	for i := range a.rows {
		k, ok := at(i)
		if !ok {
			break
		}
		a.first = append(a.first, k)
	}
	var todo []K
	for _, k := range a.first {
		if !a.current(k) {
			todo = append(todo, k)
		} else {
			a.seen.Count(obs.PrefetchCached)
		}
	}
	cached := len(a.first) - len(todo)
	if len(todo) == 0 {
		return nil
	}
	ctx, read, limited, seen := a.ctx, a.read, a.limited, a.seen
	return func() tea.Msg {
		ctx := obs.WithTrace(ctx, "prefetch.rows")
		slog.InfoContext(ctx, "prefetch", "span", "prefetch", "kind", seen.Kind(), "trigger", "rows",
			"sent", len(todo), "skipped_cached", cached)
		readAll(ctx, read, limited, seen, todo)
		return nil
	}
}

// same reports whether the first rows are those read ahead already.
func (a *Ahead[K]) same(at func(i int) (K, bool)) bool {
	n := 0
	for i := range a.rows {
		k, ok := at(i)
		if !ok {
			break
		}
		if i >= len(a.first) || a.first[i] != k {
			return false
		}
		n++
	}
	return n == len(a.first)
}

// readAll reads the details of ks, a few at a time, until ctx is done or
// GitHub reports the rate limit. Other failures are for the detail to
// report, if it is opened.
func readAll[K comparable](ctx context.Context, read func(context.Context, K) error, limited *atomic.Bool, seen *obs.Prefetched[K], ks []K) {
	sem := make(chan struct{}, aheadWorkers)
	var wg sync.WaitGroup
	defer wg.Wait()
	for i, k := range ks {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			skip(seen, obs.PrefetchCanceled, len(ks)-i)
			return
		}
		if limited.Load() {
			<-sem
			skip(seen, obs.PrefetchLimited, len(ks)-i)
			return
		}
		wg.Go(func() {
			defer func() { <-sem }()
			readOne(ctx, read, limited, seen, k)
		})
	}
}

// skip counts n reads that weren't sent, for why.
func skip[K comparable](seen *obs.Prefetched[K], why obs.PrefetchEvent, n int) {
	for range n {
		seen.Count(why)
	}
}

// readOne reads the detail of k, and records what came of it.
func readOne[K comparable](ctx context.Context, read func(context.Context, K) error, limited *atomic.Bool, seen *obs.Prefetched[K], k K) {
	seen.Count(obs.PrefetchSent)
	start := time.Now()
	err := read(ctx, k)
	outcome := "read"
	switch {
	case err == nil:
		seen.Read(k)
	case errors.Is(err, core.ErrRateLimited):
		limited.Store(true)
		outcome = "rate_limited"
		seen.Count(obs.PrefetchRateLimited)
	case ctx.Err() != nil:
		outcome = "canceled"
		seen.Count(obs.PrefetchCanceled)
	default:
		outcome = "failed"
		seen.Count(obs.PrefetchFailed)
	}
	if obs.Enabled(ctx, slog.LevelDebug) {
		slog.DebugContext(ctx, "prefetch read", "span", "prefetch", "kind", seen.Kind(), "key", fmt.Sprint(k),
			"outcome", outcome, "duration_ms", obs.Millis(time.Since(start)))
	}
}

// Moved starts the delay when the cursor moved to a row, k, whose detail
// isn't cached. ok is false when the cursor is on no row.
func (a *Ahead[K]) Moved(k K, ok bool) tea.Cmd {
	if a == nil || ok == a.hasHovered && k == a.hovered {
		return nil
	}
	a.hovered, a.hasHovered = k, ok
	a.seq++
	switch {
	case !ok:
		return nil
	case a.limited.Load():
		a.seen.Count(obs.PrefetchLimited)
		return nil
	case a.current(k):
		a.seen.Count(obs.PrefetchCached)
		return nil
	}
	msg := AheadMsg{id: a.id, seq: a.seq}
	return tea.Tick(a.delay, func(time.Time) tea.Msg { return msg })
}

// Rested reads the detail of the row the cursor rested on, unless it moved
// since. A newer read cancels an older one still in flight, so at most one
// runs.
func (a *Ahead[K]) Rested(msg AheadMsg) tea.Cmd {
	if a == nil || msg.id != a.id || msg.seq != a.seq || !a.hasHovered {
		return nil
	}
	if a.stopHover != nil {
		a.stopHover()
		a.stopHover = nil
	}
	k := a.hovered
	switch {
	case a.limited.Load():
		a.seen.Count(obs.PrefetchLimited)
		return nil
	case a.current(k):
		a.seen.Count(obs.PrefetchCached)
		return nil
	}
	ctx, cancel := context.WithCancel(obs.WithTrace(a.ctx, "prefetch.hover"))
	a.stopHover = cancel
	read, limited, seen := a.read, a.limited, a.seen
	return func() tea.Msg {
		defer cancel()
		readOne(ctx, read, limited, seen, k)
		return nil
	}
}

// Around reads the details of the n rows on each side of row i, which at
// returns by index, and false for a row not loaded: nearest first, and the
// row after before the row before, since lists are mostly read downwards.
// It cancels the reads of the last call still in flight, so call it once
// the cursor rests, and skips the rows whose details are cached.
func (a *Ahead[K]) Around(at func(i int) (K, bool), i, n int) tea.Cmd {
	if a == nil || n <= 0 {
		return nil
	}
	if a.stopAround != nil {
		a.stopAround()
		a.stopAround = nil
	}
	if a.limited.Load() {
		a.seen.Count(obs.PrefetchLimited)
		return nil
	}
	todo := make([]K, 0, 2*n)
	cached := 0
	for d := 1; d <= n; d++ {
		for _, j := range [2]int{i + d, i - d} {
			k, ok := at(j)
			switch {
			case j < 0 || !ok:
			case a.current(k):
				cached++
				a.seen.Count(obs.PrefetchCached)
			default:
				todo = append(todo, k)
			}
		}
	}
	if len(todo) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(obs.WithTrace(a.ctx, "prefetch.around"))
	a.stopAround = cancel
	read, limited, seen := a.read, a.limited, a.seen
	return func() tea.Msg {
		defer cancel()
		slog.InfoContext(ctx, "prefetch", "span", "prefetch", "kind", seen.Kind(), "trigger", "around",
			"sent", len(todo), "skipped_cached", cached)
		readAll(ctx, read, limited, seen, todo)
		return nil
	}
}
