package ui

import (
	"context"
	"log/slog"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/obs"
)

// Configure applies r, the knobs the settings resolve for the kind of item
// a reads: whether it reads at all, the window around the cursor, and how
// long the cursor rests first. They count from the next move of the
// cursor; turning the reads off stops those in flight.
func (a *Ahead[K]) Configure(r config.Resolved) {
	if a == nil {
		return
	}
	a.span, a.delay = config.Window{Before: max(r.Window.Before, 0), After: max(r.Window.After, 0)}, max(r.Rest, 0)
	wasOff := a.off
	a.off = !r.Enabled
	if a.off && !wasOff {
		a.Reset(a.parent)
	}
}

// On reports whether a reads ahead, as the settings say.
func (a *Ahead[K]) On() bool {
	return a != nil && !a.off
}

// Window tells a that the cursor is on row i of the list, which at returns
// by index, and false for a row not loaded or with no detail to read, such
// as a directory. Once the cursor rests there for the rest that
// [Ahead.Configure] set, a reads the row under it, then the rows of the
// window below and above it: nearest first, and below before above, since
// lists are mostly read downwards. A row with no detail counts toward the
// window, and costs nothing; a row that at returns twice is read once.
//
// Pass a nil at, or an i below 0, while the list hasn't loaded or the
// cursor is on no row, such as while the list doesn't have the focus,
// which drops a rest still to come. The first call with an at after
// [Ahead.Reset] is the list as it first shows: its window is read at once,
// since the list loading counts as a rest, even if it has nothing to read,
// as when its first rows are all folders.
//
// Call it whenever the cursor may have moved or the list changed; it
// waits again only when the rows of the window change, or when the end of
// its rest never came back, as for a modal hidden then. Each rest reads the
// window again, skipping the rows cached or still being read, and those
// whose read ahead failed lately. The reads of the last rest go on for the
// rows still in the window, and stop for those that left it.
func (a *Ahead[K]) Window(at func(i int) (K, bool), i int) tea.Cmd {
	if a == nil || a.off {
		return nil
	}
	rows := windowRows(at, i, a.span.Before, a.span.After)
	same := a.windowed && slices.Equal(rows, a.around)
	// A rest whose end never came back, because the modal was hidden then
	// and got no messages, is waited for again. A message handled after the
	// timer fired but before its AheadMsg is delivered looks the same, so the
	// rest keeps its seq: whichever end arrives first reads, the other finds
	// the rest consumed.
	lost := same && !a.restEnds.IsZero() && !time.Now().Before(a.restEnds)
	if same && !lost {
		return nil
	}
	a.around, a.windowed = rows, true
	a.restEnds = time.Time{}
	if !lost {
		a.seq++
	}
	if at == nil || i < 0 {
		// The list lost the focus or its rows: the reads of the rows that
		// were in the window stop now, rather than at a rest that may
		// never come.
		a.cancelLeft()
		return nil
	}
	if !a.loaded {
		a.loaded = true
		return a.readWindow()
	}
	if len(rows) == 0 {
		return nil
	}
	if a.halt() {
		return nil
	}
	msg := AheadMsg{id: a.id, seq: a.seq}
	a.restEnds = time.Now().Add(a.delay)
	return tea.Tick(a.delay, func(time.Time) tea.Msg { return msg })
}

// Rest has the first window after a [Ahead.Reset] wait for the rest too,
// rather than being read at once as the first window of a list that shows
// is. Call it after Reset, for what reads ahead of a single item that the
// user is already on, such as the files of the pull request a modal
// shows, which the user may leave at once.
func (a *Ahead[K]) Rest() {
	if a != nil {
		a.loaded = true
	}
}

// Keep has the read ahead of k go on though k leaves the window, such as
// the row the cursor just landed on, which the caller reads itself and
// leaves out of the window: the caller's read then joins the one in
// flight rather than starting over once a rest stops it. Only the last
// row kept is; a row kept before stops at the next rest if it left the
// window.
func (a *Ahead[K]) Keep(k K) {
	if a != nil {
		a.keep, a.kept = k, true
	}
}

// Unkeep has the read ahead of the row kept stop at the next rest, if it
// left the window, as any other row's does.
func (a *Ahead[K]) Unkeep() {
	if a != nil {
		a.kept = false
	}
}

// Rested reads the window the cursor rested on, unless it moved since or an
// end of the same rest already read it.
func (a *Ahead[K]) Rested(msg AheadMsg) tea.Cmd {
	if a == nil || msg.id != a.id || msg.seq != a.seq || !a.windowed || a.restEnds.IsZero() {
		return nil
	}
	a.restEnds = time.Time{}
	return a.readWindow()
}

// windowRows returns the rows of the window around row i that at has, each
// once: the row under the cursor first, then nearest first, below before
// above. A nil at or an i below 0 has none.
func windowRows[K comparable](at func(i int) (K, bool), i, before, after int) []K {
	if at == nil || i < 0 {
		return nil
	}
	rows := make([]K, 0, 1+before+after)
	add := func(j int) {
		if j < 0 {
			return
		}
		if k, ok := at(j); ok && !slices.Contains(rows, k) {
			rows = append(rows, k)
		}
	}
	add(i)
	for d := 1; d <= max(before, after); d++ {
		if d <= after {
			add(i + d)
		}
		if d <= before {
			add(i - d)
		}
	}
	return rows
}

// readWindow reads the rows of the window that aren't cached, being read,
// or failed lately (aheadFailedFor). The reads of the last window go on
// for the rows still in it, and stop for those that left it.
func (a *Ahead[K]) readWindow() tea.Cmd {
	a.cancelLeft()
	if a.halt() {
		return nil
	}
	return a.readRows()
}

// cancelLeft stops the reads of the rows that left the window, and those
// whose read ended.
func (a *Ahead[K]) cancelLeft() {
	for k, cancel := range a.reading {
		left := !slices.Contains(a.around, k) && (!a.kept || k != a.keep)
		if left || !a.flying.has(k) {
			cancel()
			delete(a.reading, k)
		}
		if left {
			// So that it is read again at once if it comes back before
			// its read unwinds.
			a.flying.drop(k)
		}
	}
}

// readRows reads the rows of the window that need it.
func (a *Ahead[K]) readRows() tea.Cmd {
	todo := make([]K, 0, len(a.around))
	cached, failed := 0, 0
	now := time.Now()
	for _, k := range a.around {
		switch {
		case a.current(k):
			cached++
			a.seen.Count(obs.PrefetchCached)
		case a.flying.has(k):
		case a.failed.recent(k, now):
			failed++
		default:
			todo = append(todo, k)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	ctx := obs.WithTrace(a.ctx, "prefetch.window")
	ctxs := make(map[K]context.Context, len(todo))
	for _, k := range todo {
		ctxs[k], a.reading[k] = context.WithCancel(ctx)
	}
	r := a.start(todo)
	return func() tea.Msg {
		slog.InfoContext(ctx, "prefetch", "span", "prefetch", "kind", r.seen.Kind(), "trigger", "window",
			"sent", len(todo), "skipped_cached", cached, "skipped_failed", failed)
		r.readAll(ctx, todo, func(k K) context.Context { return ctxs[k] })
		return nil
	}
}
