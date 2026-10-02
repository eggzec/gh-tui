package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// Ahead reads the details of the rows of a list before they are opened, into
// the cache the detail reads from, so that they open at once. K identifies
// a row, such as the query of its first comments. Create it with
// [NewAhead]; a nil *Ahead reads nothing.
//
// [Ahead.Window] reads the row under the cursor and a window of rows
// around it, each time the cursor rests, as the settings that
// [Ahead.Configure] applies say.
//
// Each read costs requests, so few run at once, and the reads stop once
// GitHub reports the rate limit, until [Ahead.Resume], and once the reads
// ahead spent their budget, until the GraphQL quota refills
// (obs.PrefetchSpent). While a detail the user opened loads, reads wait
// to start ([Ahead.Pause]).
type Ahead[K comparable] struct {
	id int64
	// seen remembers what was read, so that opening it counts as a use,
	// under the kind of detail, such as pull.
	seen *obs.Prefetched[K]
	// delay is how long the cursor rests on a row before its window is
	// read.
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
	// failed holds the rows whose read ahead failed lately, so that a row
	// that keeps failing isn't sent again on every rest.
	failed *failures[K]
	// pause holds the reads that haven't started while it is closed.
	pause *gate
	// overBudget is set once a read was skipped for the budget, which
	// counts only the first.
	overBudget bool

	// seq counts the times the window moved, so that only the latest
	// delay fires.
	seq int

	// slots bound the reads in flight, with those of the Aheads that share
	// them.
	slots *Slots
	// parent bounds the reads of the list shown, as Reset was given it.
	parent context.Context

	// span is the window that Configure set, and off says the settings
	// turned the reads off.
	span config.Window
	off  bool
	// around holds the rows of the window around the cursor that have a
	// detail to read, the row under the cursor first, and windowed says
	// that the next rest reads them; until it is set again, the next call
	// to Window waits for a rest even if the window didn't move.
	around   []K
	windowed bool
	// loaded is set once the list showed since it was reset: the first
	// window of a list is read at once, as if the cursor had rested.
	loaded bool
	// reading cancels each read of a window still in flight, by row, so
	// that a rest stops only the reads of the rows that left the window.
	reading map[K]context.CancelFunc
	// keep is the row whose read goes on though it left the window, if
	// kept is set ([Ahead.Keep]).
	keep K
	kept bool
}

// AheadMsg reports that the cursor rested on a row. Sections pass it to
// [Ahead.Rested].
type AheadMsg struct {
	id  int64
	seq int
}

var lastAhead atomic.Int64

// NewAhead returns an Ahead that reads the detail of a row with read and
// asks current whether it is cached already. It reads nothing until
// [Ahead.Configure] says which rows to read. Kind names the detail in the
// log, such as pull.
func NewAhead[K comparable](kind string, read func(ctx context.Context, k K) error, current func(k K) bool) *Ahead[K] {
	return &Ahead[K]{
		id:      lastAhead.Add(1),
		seen:    obs.NewPrefetched[K](kind),
		read:    read,
		current: current,
		ctx:     obs.ForPrefetch(context.Background()),
		cancel:  func() {},
		limited: new(atomic.Bool),
		flying:  newFlights[K](),
		failed:  newFailures[K](),
		pause:   newGate(),
		// Until it shares the session's ([Ahead.Share]).
		slots:   NewSlots(config.Default().Prefetch.Parallel),
		parent:  context.Background(),
		reading: make(map[K]context.CancelFunc),
	}
}

// Share bounds the reads of a, from its next ones, with those of every
// Ahead that shares s, so that they keep to s together.
func (a *Ahead[K]) Share(s *Slots) {
	if a != nil && s != nil {
		a.slots = s
	}
}

// Reset cancels the reads of the list shown, for a new list whose reads
// parent bounds.
func (a *Ahead[K]) Reset(parent context.Context) {
	if a == nil {
		return
	}
	a.cancel()
	a.parent = parent
	a.ctx, a.cancel = context.WithCancel(obs.ForPrefetch(parent))
	a.flying.clear()
	// A new list tries its rows again. Reads of the last list still
	// unwinding note their failures in the old set, which nothing reads.
	a.failed = newFailures[K]()
	a.seq++
	// Cancelling ctx cancelled the reads of the window.
	clear(a.reading)
	a.around, a.windowed, a.loaded = a.around[:0], false, false
	var zero K
	a.keep, a.kept = zero, false
}

// Opened records that the detail of k was opened, so that the summary
// counts it as a use if it was read ahead.
func (a *Ahead[K]) Opened(k K) {
	if a != nil {
		a.seen.Opened(k)
	}
}

// Resume reads ahead again after GitHub reported the rate limit, such as
// for another repository, and tries again the rows whose reads failed
// lately, such as when the user refreshes the list.
func (a *Ahead[K]) Resume() {
	if a != nil {
		a.limited.Store(false)
		a.failed = newFailures[K]()
		// The window the rate limit stopped is read at the next call,
		// even if the cursor didn't move.
		a.windowed = false
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

// Pauser holds reads ahead while a detail the user opened loads, as
// [Ahead.Pause] does.
type Pauser interface {
	Pause() (resume func())
}

// PauseAll pauses each of ps that isn't nil, and returns the func that
// resumes them all, which may be called more than once.
func PauseAll(ps ...Pauser) (resume func()) {
	resumes := make([]func(), 0, len(ps))
	for _, p := range ps {
		if p != nil {
			resumes = append(resumes, p.Pause())
		}
	}
	return func() {
		for _, r := range resumes {
			r()
		}
	}
}

// start marks ks as being read by a new batch of reads, which it returns.
func (a *Ahead[K]) start(ks []K) batch[K] {
	id := a.flying.start(ks)
	for _, k := range ks {
		a.seen.Started(k)
	}
	return batch[K]{id: id, read: a.read, current: a.current, limited: a.limited, seen: a.seen, flying: a.flying, failed: a.failed, pause: a.pause, slots: a.slots}
}

// halted reports why reads ahead stop, if they do: GitHub reported the
// rate limit, or the reads ahead of the session spent their budget.
func halted(limited *atomic.Bool) (obs.PrefetchEvent, bool) {
	switch {
	case limited.Load():
		return obs.PrefetchLimited, true
	case obs.PrefetchSpent():
		return obs.PrefetchOverBudget, true
	}
	return 0, false
}

// halt reports whether reads ahead stop, and counts a read skipped for
// why: each one for the rate limit, and only the first for the budget
// until it comes back, since the cursor moving over the rows of a list
// that reads nothing ahead any more skips nothing new.
func (a *Ahead[K]) halt() bool {
	why, ok := halted(a.limited)
	switch {
	case !ok:
		a.overBudget = false
		return false
	case why == obs.PrefetchOverBudget && a.overBudget:
		return true
	}
	a.overBudget = why == obs.PrefetchOverBudget
	a.seen.Count(why)
	logSkipped(a.ctx, a.seen.Kind(), why, 1)
	return true
}

// logSkipped logs, at debug level, that n reads ahead of kind weren't
// sent, and why, which the summary only counts.
func logSkipped(ctx context.Context, kind string, why obs.PrefetchEvent, n int) {
	if n == 0 || !obs.Enabled(ctx, slog.LevelDebug) {
		return
	}
	slog.DebugContext(ctx, "prefetch skipped", "span", "prefetch", "kind", kind, "why", skipReason(why), "count", n)
}

// skipReason names why, a reason not to read ahead, as the summary does.
func skipReason(why obs.PrefetchEvent) string {
	if r, ok := skipReasons[why]; ok {
		return r
	}
	return "other"
}

var skipReasons = map[obs.PrefetchEvent]string{
	obs.PrefetchCached:     "cached",
	obs.PrefetchLimited:    "limit",
	obs.PrefetchCanceled:   "canceled",
	obs.PrefetchOverBudget: "budget",
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
	failed  *failures[K]
	pause   *gate
	slots   *Slots
}

// readAll reads the details of ks, as many at a time as the slots let,
// until ctx is done or GitHub reports the rate limit. rowCtx, if not nil,
// returns the context of the read of one row, which may end before ctx.
// Other failures are for the detail to report, if it is opened.
func (b batch[K]) readAll(ctx context.Context, ks []K, rowCtx func(K) context.Context) {
	var wg sync.WaitGroup
	defer wg.Wait()
	for i, k := range ks {
		kctx := ctx
		if rowCtx != nil {
			kctx = rowCtx(k)
		}
		// A row that left the window doesn't wait for a slot.
		if kctx.Err() != nil && ctx.Err() == nil {
			b.skip(ctx, obs.PrefetchCanceled, []K{k})
			continue
		}
		held, waited, err := b.take(ctx)
		if err != nil {
			b.skip(ctx, obs.PrefetchCanceled, ks[i:])
			return
		}
		if why, ok := halted(b.limited); ok {
			b.slots.release()
			b.skip(ctx, why, ks[i:])
			return
		}
		wg.Go(func() {
			defer b.slots.release()
			b.send(kctx, k, held, waited)
		})
	}
}

// take waits until no pause holds the reads and a slot is free, and takes
// the slot. It reports whether a pause held it, and for how long. A
// paused read gives its slot back, so that it doesn't hold up the reads of
// the other pages and kinds that share the slots.
func (b batch[K]) take(ctx context.Context) (held bool, waited time.Duration, err error) {
	for {
		h, w, err := b.pause.wait(ctx)
		held, waited = held || h, waited+w
		if err != nil {
			return held, waited, err
		}
		if err := b.slots.acquire(ctx); err != nil {
			return held, waited, err
		}
		if !b.pause.holding() {
			return held, waited, nil
		}
		// A pause began while it waited for the slot.
		b.slots.release()
	}
}

// skip counts the reads of ks that weren't sent, for why, and ends them.
func (b batch[K]) skip(ctx context.Context, why obs.PrefetchEvent, ks []K) {
	logSkipped(ctx, b.seen.Kind(), why, len(ks))
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

// send reads the detail of k, unless it was read while it waited, once a
// pause held it for waited if held, and records what came of it.
func (b batch[K]) send(ctx context.Context, k K, held bool, waited time.Duration) {
	// Such as a row that left the window while it waited.
	if ctx.Err() != nil {
		b.skip(ctx, obs.PrefetchCanceled, []K{k})
		return
	}
	// Reads that ended meanwhile may have met the rate limit or spent the
	// budget.
	if why, ok := halted(b.limited); ok {
		b.skip(ctx, why, []K{k})
		return
	}
	if held && obs.Enabled(ctx, slog.LevelDebug) {
		slog.DebugContext(ctx, "prefetch paused", "span", "prefetch", "kind", b.seen.Kind(), "key", obs.LogKey(fmt.Sprint(k)),
			"waited_ms", obs.Millis(waited))
	}
	// What it waited for, such as a pause for the detail the user opened,
	// or a slot, may have let something else read it meanwhile.
	if b.current(k) {
		b.skip(ctx, obs.PrefetchCached, []K{k})
		return
	}
	b.seen.Count(obs.PrefetchSent)
	start := time.Now()
	err := b.read(ctx, k)
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
		// A read that got no answer, or only a server error, may well
		// succeed once GitHub answers again, which OnlineMsg tells of.
		if !Unreached(err) {
			b.failed.add(k, time.Now())
		}
	}
	b.end(k, err == nil)
	if obs.Enabled(ctx, slog.LevelDebug) {
		slog.DebugContext(ctx, "prefetch read", "span", "prefetch", "kind", b.seen.Kind(), "key", obs.LogKey(fmt.Sprint(k)),
			"outcome", outcome, "duration_ms", obs.Millis(time.Since(start)))
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

// drop forgets the read of k, whichever batch reads it.
func (f *flights[K]) drop(k K) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, k)
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

// aheadFailedFor is how long a row whose read ahead failed isn't read
// ahead again. A read that fails, such as for a row GitHub no longer has
// or one the token can't see, mostly fails again, while the cursor rests
// near it many times a minute.
const aheadFailedFor = time.Minute

// failures holds the rows whose read ahead failed, each until it may be
// read ahead again. The reads end in commands, so it is safe for
// concurrent use.
type failures[K comparable] struct {
	mu    sync.Mutex
	until map[K]time.Time
}

func newFailures[K comparable]() *failures[K] { return &failures[K]{until: make(map[K]time.Time)} }

// add notes that the read of k failed at now, and forgets the rows whose
// span has passed.
func (f *failures[K]) add(k K, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for r, until := range f.until {
		if !now.Before(until) {
			delete(f.until, r)
		}
	}
	f.until[k] = now.Add(aheadFailedFor)
}

// recent reports whether the read of k failed within the span before now.
func (f *failures[K]) recent(k K, now time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	until, ok := f.until[k]
	return ok && now.Before(until)
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

// holding reports whether a holder holds g.
func (g *gate) holding() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.holders > 0
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
