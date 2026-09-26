package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
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
// GitHub reports the rate limit, until [Ahead.Resume]. While a detail the
// user opened loads, reads wait to start ([Ahead.Pause]).
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
	// flying holds the rows being read, so that a row whose read is in
	// flight isn't read again meanwhile.
	flying *flights[K]
	// pause holds the reads that haven't started while it is closed.
	pause *gate
	// first are the first rows read ahead for the list.
	first []K

	// hovered is the row the cursor was last seen on, and seq counts the
	// times it moved, so that only the latest delay fires.
	hovered    K
	hasHovered bool
	seq        int
	stopHover  func()
	// stopAround cancels the reads around the cursor still in flight.
	stopAround func()
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
		flying:  newFlights[K](),
		pause:   newGate(),
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
	a.flying.clear()
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

// Pause holds the reads that haven't started, such as while a detail the
// user opened loads, so that they don't compete with its requests, until
// the returned resume is called; resume may be called more than once.
// Reads in flight go on. While any of several pauses holds, reads wait.
func (a *Ahead[K]) Pause() (resume func()) {
	if a == nil {
		return func() {}
	}
	return a.pause.close()
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
	cached := 0
	for _, k := range a.first {
		switch {
		case a.current(k):
			cached++
			a.seen.Count(obs.PrefetchCached)
		case !a.flying.has(k):
			todo = append(todo, k)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	r := a.start(todo)
	ctx := a.ctx
	return func() tea.Msg {
		ctx := obs.WithTrace(ctx, "prefetch.rows")
		slog.InfoContext(ctx, "prefetch", "span", "prefetch", "kind", r.seen.Kind(), "trigger", "rows",
			"sent", len(todo), "skipped_cached", cached)
		r.readAll(ctx, todo)
		return nil
	}
}

// start marks ks as being read by a new batch of reads, which it returns.
func (a *Ahead[K]) start(ks []K) batch[K] {
	id := a.flying.start(ks)
	for _, k := range ks {
		a.seen.Started(k)
	}
	return batch[K]{id: id, read: a.read, current: a.current, limited: a.limited, seen: a.seen, flying: a.flying, pause: a.pause}
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

// batch is a group of reads started at once, which ends them. It only
// holds what never changes or is safe for concurrent use, so it runs in a
// command.
type batch[K comparable] struct {
	id      int
	read    func(context.Context, K) error
	current func(K) bool
	limited *atomic.Bool
	seen    *obs.Prefetched[K]
	flying  *flights[K]
	pause   *gate
}

// readAll reads the details of ks, a few at a time, until ctx is done or
// GitHub reports the rate limit. Other failures are for the detail to
// report, if it is opened.
func (b batch[K]) readAll(ctx context.Context, ks []K) {
	sem := make(chan struct{}, aheadWorkers)
	var wg sync.WaitGroup
	defer wg.Wait()
	for i, k := range ks {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			b.skip(obs.PrefetchCanceled, ks[i:])
			return
		}
		if b.limited.Load() {
			<-sem
			b.skip(obs.PrefetchLimited, ks[i:])
			return
		}
		wg.Go(func() {
			defer func() { <-sem }()
			b.readOne(ctx, k)
		})
	}
}

// skip counts the reads of ks that weren't sent, for why, and ends them.
func (b batch[K]) skip(why obs.PrefetchEvent, ks []K) {
	for _, k := range ks {
		b.seen.Count(why)
		b.end(k, false)
	}
}

// end ends the read of k, which brought it if ok.
func (b batch[K]) end(k K, ok bool) {
	b.flying.end(b.id, k)
	if ok {
		b.seen.Read(k)
	} else {
		b.seen.Dropped(k)
	}
}

// readOne reads the detail of k once no pause holds it, and records what
// came of it.
func (b batch[K]) readOne(ctx context.Context, k K) {
	held, waited, err := b.pause.wait(ctx)
	if err != nil {
		b.skip(obs.PrefetchCanceled, []K{k})
		return
	}
	if held {
		if obs.Enabled(ctx, slog.LevelDebug) {
			slog.DebugContext(ctx, "prefetch paused", "span", "prefetch", "kind", b.seen.Kind(), "key", fmt.Sprint(k),
				"waited_ms", obs.Millis(waited))
		}
		// What paused it, such as the detail the user opened, may have
		// read it meanwhile.
		if b.current(k) {
			b.skip(obs.PrefetchCached, []K{k})
			return
		}
	}
	b.seen.Count(obs.PrefetchSent)
	start := time.Now()
	err = b.read(ctx, k)
	outcome := "read"
	switch {
	case err == nil:
	case errors.Is(err, core.ErrRateLimited):
		b.limited.Store(true)
		outcome = "rate_limited"
		b.seen.Count(obs.PrefetchRateLimited)
	case ctx.Err() != nil:
		outcome = "canceled"
		b.seen.Count(obs.PrefetchCanceled)
	default:
		outcome = "failed"
		b.seen.Count(obs.PrefetchFailed)
	}
	b.end(k, err == nil)
	if obs.Enabled(ctx, slog.LevelDebug) {
		slog.DebugContext(ctx, "prefetch read", "span", "prefetch", "kind", b.seen.Kind(), "key", fmt.Sprint(k),
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
	case a.flying.has(k):
		// Such as a first row, read since the list loaded.
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
	case a.flying.has(k):
		return nil
	}
	ctx, cancel := context.WithCancel(obs.WithTrace(a.ctx, "prefetch.hover"))
	r := a.start([]K{k})
	a.stopHover = r.stop(cancel)
	return func() tea.Msg {
		defer cancel()
		r.readOne(ctx, k)
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
			case !a.flying.has(k):
				todo = append(todo, k)
			}
		}
	}
	if len(todo) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(obs.WithTrace(a.ctx, "prefetch.around"))
	r := a.start(todo)
	a.stopAround = r.stop(cancel)
	return func() tea.Msg {
		defer cancel()
		slog.InfoContext(ctx, "prefetch", "span", "prefetch", "kind", r.seen.Kind(), "trigger", "around",
			"sent", len(todo), "skipped_cached", cached)
		r.readAll(ctx, todo)
		return nil
	}
}

// stop returns a func that cancels the reads of b with cancel, and forgets
// them as in flight at once, so that a row they were reading can be read
// again before they unwind.
func (b batch[K]) stop(cancel context.CancelFunc) func() {
	return func() {
		cancel()
		b.flying.forget(b.id)
	}
}

// flights holds the rows being read, by the batch that reads them. The
// reads end in commands, so it is safe for concurrent use.
type flights[K comparable] struct {
	mu   sync.Mutex
	rows map[K]int
	last int
}

func newFlights[K comparable]() *flights[K] { return &flights[K]{rows: make(map[K]int)} }

// start marks ks as read by a new batch, and returns its id.
func (f *flights[K]) start(ks []K) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last++
	for _, k := range ks {
		f.rows[k] = f.last
	}
	return f.last
}

// end forgets the read of k by batch id, unless another batch reads k since.
func (f *flights[K]) end(id int, k K) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rows[k] == id {
		delete(f.rows, k)
	}
}

// forget forgets the reads of batch id.
func (f *flights[K]) forget(id int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	maps.DeleteFunc(f.rows, func(_ K, b int) bool { return b == id })
}

// has reports whether k is being read.
func (f *flights[K]) has(k K) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.rows[k]
	return ok
}

// clear forgets every read.
func (f *flights[K]) clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.rows)
}

// gate holds whoever waits on it while it is closed, by one or more
// holders at once. It is safe for concurrent use.
type gate struct {
	mu      sync.Mutex
	holders int
	// open is closed while no holder holds the gate.
	open chan struct{}
}

func newGate() *gate {
	open := make(chan struct{})
	close(open)
	return &gate{open: open}
}

// close closes g until the returned func is called, once or more.
func (g *gate) close() func() {
	g.mu.Lock()
	if g.holders == 0 {
		g.open = make(chan struct{})
	}
	g.holders++
	g.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.holders--
			if g.holders == 0 {
				close(g.open)
			}
		})
	}
}

// wait waits until g is open or ctx is done. It reports whether g held it,
// and for how long.
func (g *gate) wait(ctx context.Context) (held bool, waited time.Duration, err error) {
	g.mu.Lock()
	open := g.open
	g.mu.Unlock()
	select {
	case <-open:
		return false, 0, nil
	default:
	}
	start := time.Now()
	select {
	case <-open:
	case <-ctx.Done():
		err = ctx.Err()
	}
	return true, time.Since(start), err
}
