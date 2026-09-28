package ui

import (
	"context"
	"errors"
	"log/slog"
	"sync"
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
// Each page costs a request, and many visits never switch filters, so the
// pages of a repository are read only once the user switched filters there
// ([Filters.Arm]), and once a session. They are read one at a time, and
// the rest are skipped once GitHub reports the rate limit or the reads
// ahead spent their budget (obs.PrefetchSpent).
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
	// scope names the repository shown, and armed holds the scopes where
	// the user switched filters. reading is set while the pages of scope
	// are read ahead.
	scope   string
	armed   map[string]bool
	reading bool
	// done holds the scopes whose pages were all read ahead. The reads set
	// it, in their command, so mu guards it.
	mu   sync.Mutex
	done map[string]bool
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
		ctx:    obs.ForPrefetch(context.Background()),
		cancel: func() {},
		armed:  make(map[string]bool),
		done:   make(map[string]bool),
	}
}

// Reset cancels the reads in flight, for the repository that scope names,
// whose reads parent bounds.
func (f *Filters[Q]) Reset(parent context.Context, scope string) {
	if f == nil {
		return
	}
	f.cancel()
	f.ctx, f.cancel = context.WithCancel(obs.ForPrefetch(parent))
	f.scope, f.reading = scope, false
}

// Arm records that the user switched filters in the repository shown, which
// lets [Filters.Read] read its other filters ahead.
func (f *Filters[Q]) Arm() {
	if f != nil {
		f.armed[f.scope] = true
	}
}

// Read reads the first pages of the queries others returns ahead, in
// order, the first time it is called once the repository shown is armed;
// others is only called then. Reads that a Reset cancelled are tried
// again the next time. Call it once the list shown has loaded, so that its
// own reads go first.
func (f *Filters[Q]) Read(others func() []Q) tea.Cmd {
	if f == nil || !f.armed[f.scope] || f.reading || f.isDone(f.scope) {
		return nil
	}
	f.reading = true
	ctx, scope, qs := f.ctx, f.scope, others()
	if len(qs) == 0 {
		f.setDone(scope)
		return nil
	}
	return func() tea.Msg {
		f.readAll(obs.WithTrace(ctx, "prefetch.filters"), qs)
		if ctx.Err() == nil {
			f.setDone(scope)
		}
		return nil
	}
}

func (f *Filters[Q]) isDone(scope string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.done[scope]
}

func (f *Filters[Q]) setDone(scope string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.done[scope] = true
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
		case obs.PrefetchSpent():
			f.decided(ctx, q, "skipped_budget", obs.PrefetchOverBudget)
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
	f.seen.Started(q)
	start := time.Now()
	err := f.read(ctx, q)
	level, outcome := slog.LevelInfo, "read"
	if err == nil {
		f.seen.Read(q)
	} else {
		f.seen.Dropped(q)
	}
	switch {
	case err == nil:
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
		attrs = append(attrs, "err", err.Error())
	}
	slog.Log(ctx, level, "prefetch filter", attrs...)
	return limited
}
