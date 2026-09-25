package ui

import (
	"context"
	"errors"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// Filters reads ahead the first page of each filter of a list other than
// the one shown, such as the closed and merged pull requests while the open
// ones are, into the cache the list reads from, so that switching to one
// shows it at once. Q is the query of a first page. Create it with
// [NewFilters]; a nil *Filters reads nothing.
//
// Each page costs a request, so the pages are read one at a time, once per
// [Filters.Reset], and the rest are skipped once GitHub reports the rate
// limit.
type Filters[Q comparable] struct {
	// seen remembers what was read, so that switching to it counts as a
	// use, under the kind of list, such as pull_filter.
	seen *obs.Prefetched[Q]
	// fresh reports whether the page of q can be read without a request.
	// It may do I/O, since it runs in the command.
	fresh func(q Q) bool
	read  func(ctx context.Context, q Q) error
	// name names the filter of q in the log, such as closed.
	name func(q Q) string

	// ctx bounds the reads of the repository shown, and cancel ends them.
	ctx    context.Context
	cancel context.CancelFunc
	// started is set once the pages have been read ahead since Reset.
	started bool
}

// NewFilters returns a Filters that reads a first page with read, unless
// fresh reports that it can be read without a request. Kind names the list
// in the log and the summary, and name the filter of a query.
func NewFilters[Q comparable](kind string, read func(ctx context.Context, q Q) error, fresh func(q Q) bool, name func(q Q) string) *Filters[Q] {
	return &Filters[Q]{
		seen:   obs.NewPrefetched[Q](kind),
		fresh:  fresh,
		read:   read,
		name:   name,
		ctx:    context.Background(),
		cancel: func() {},
	}
}

// Reset cancels the reads in flight, for another repository whose reads
// parent bounds, and lets [Filters.Read] read ahead again.
func (f *Filters[Q]) Reset(parent context.Context) {
	if f == nil {
		return
	}
	f.cancel()
	f.ctx, f.cancel = context.WithCancel(parent)
	f.started = false
}

// Read reads the first pages of the queries others returns ahead, in
// order, the first time it is called after a Reset; others is only called
// then. Call it once the list shown has loaded, so that its own reads go
// first.
func (f *Filters[Q]) Read(others func() []Q) tea.Cmd {
	if f == nil || f.started {
		return nil
	}
	f.started = true
	ctx, qs := f.ctx, others()
	if len(qs) == 0 {
		return nil
	}
	return func() tea.Msg {
		f.readAll(obs.WithTrace(ctx, "prefetch.filters"), qs)
		return nil
	}
}

// Opened records that the list of q is shown, so that the summary counts it
// as a use if it was read ahead.
func (f *Filters[Q]) Opened(q Q) {
	if f != nil {
		f.seen.Opened(q)
	}
}

// readAll reads the pages of qs one by one, until ctx is done or GitHub
// reports the rate limit. It only reads the fields set in NewFilters, which
// never change, so it is safe to run in a command.
func (f *Filters[Q]) readAll(ctx context.Context, qs []Q) {
	limited := false
	for _, q := range qs {
		switch {
		case ctx.Err() != nil:
			f.decided(ctx, q, "canceled", obs.PrefetchCanceled)
		case limited:
			f.decided(ctx, q, "skipped_rate_limited", obs.PrefetchLimited)
		case f.fresh(q):
			f.decided(ctx, q, "skipped_cached", obs.PrefetchCached)
		default:
			limited = f.readOne(ctx, q)
		}
	}
}

// decided logs and counts a page that wasn't read, for why.
func (f *Filters[Q]) decided(ctx context.Context, q Q, why string, e obs.PrefetchEvent) {
	f.seen.Count(e)
	slog.InfoContext(ctx, "prefetch filter", "span", "prefetch", "kind", f.seen.Kind(), "filter", f.name(q),
		"decision", why)
}

// readOne reads the page of q, and records what came of it. It reports
// whether GitHub refused it with the rate limit.
func (f *Filters[Q]) readOne(ctx context.Context, q Q) (limited bool) {
	f.seen.Count(obs.PrefetchSent)
	start := time.Now()
	err := f.read(ctx, q)
	level, outcome := slog.LevelInfo, "read"
	switch {
	case err == nil:
		f.seen.Read(q)
	case errors.Is(err, core.ErrRateLimited):
		level, outcome, limited = slog.LevelWarn, "rate_limited", true
		f.seen.Count(obs.PrefetchRateLimited)
	case ctx.Err() != nil:
		outcome = "canceled"
		f.seen.Count(obs.PrefetchCanceled)
	default:
		level, outcome = slog.LevelWarn, "failed"
		f.seen.Count(obs.PrefetchFailed)
	}
	attrs := []any{"span", "prefetch", "kind", f.seen.Kind(), "filter", f.name(q),
		"decision", "sent", "outcome", outcome, "duration_ms", obs.Millis(time.Since(start))}
	if err != nil && outcome == "failed" {
		attrs = append(attrs, "err", err)
	}
	slog.Log(ctx, level, "prefetch filter", attrs...)
	return limited
}
