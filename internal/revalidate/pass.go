package revalidate

import (
	"cmp"
	"context"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// Pass is what one pass did, such as for a log.
type Pass struct {
	Start, End time.Time
	// Listed counts the entries the sources listed, and Due those in
	// scope that weren't fresh.
	Listed, Due int
	// Budget is how many requests the pass could send: what the budget
	// allows in an interval.
	Budget int
	// Sent counts the requests sent, and the rest what the checks found.
	Sent, NotModified, Changed, Gone, Failed, Skipped int
	// Deferred counts the due entries left for a later pass, because the
	// budget didn't allow them in this one or the pass stopped early.
	Deferred int
	// Offline reports that the pass stopped because GitHub couldn't be
	// reached, and RetryAt, if set, that a rate limit stopped it until
	// then.
	Offline bool
	RetryAt time.Time
	// Published holds the sync keys of the changes, in the order they
	// were published.
	Published []string
}

// pass checks the entries that are due, most important first, and returns
// what it did.
func (r *Revalidator) pass(ctx context.Context) Pass {
	p := Pass{Start: time.Now()}
	lists := make([][]Entry, len(r.sources))
	for i, src := range r.sources {
		lists[i] = src()
	}
	all := slices.Concat(lists...)
	p.Listed = len(all)
	first, rest := r.due(all, p.Start)
	p.Due = len(first) + len(rest)

	// A pass sends what the budget allows in an interval, so that one over
	// many entries doesn't hold back the next, which starts over from the
	// most important ones.
	quota := max(1, int(int64(r.cfg.budget)*int64(r.cfg.interval)/int64(time.Minute)))
	p.Budget = quota
	if n := len(first) + len(rest); n > quota {
		p.Deferred += n - quota
		if len(first) >= quota {
			first, rest = first[:quota], nil
		} else {
			rest = rest[:quota-len(first)]
		}
	}
	// The selected repository is on screen, so its changes are shown as
	// soon as its entries are checked.
	if r.check(ctx, first, &p) {
		r.check(ctx, rest, &p)
	} else {
		p.Deferred += len(rest)
	}
	p.End = time.Now()
	return p
}

// due returns the entries to check among all at now: first those of the
// selected repository and those of none in scope, then the rest in scope,
// each most recently used first. Entries that are fresh aren't due.
func (r *Revalidator) due(all []Entry, now time.Time) (first, rest []Entry) {
	repo := r.selected()
	r.mu.Lock()
	for id, at := range r.checked {
		if now.Sub(at) >= r.cfg.freshFor {
			delete(r.checked, id)
		}
	}
	checked := maps.Clone(r.checked)
	r.mu.Unlock()

	for _, e := range all {
		if now.Sub(e.CheckedAt) < r.cfg.freshFor {
			continue
		}
		if _, ok := checked[e.ID]; ok {
			continue
		}
		// Entries of no repository, such as the inbox, are on every
		// screen, but those not used lately, such as a filter tried once,
		// are left out like any other.
		inScope := r.cfg.scope == ScopeAll || now.Sub(e.UsedAt) < r.cfg.recent
		none := e.Repo == (core.RepoRef{})
		switch {
		case none && inScope, !none && sameRepo(e.Repo, repo):
			first = append(first, e)
		case inScope:
			rest = append(rest, e)
		}
	}
	byUse := func(a, b Entry) int {
		return cmp.Or(b.UsedAt.Compare(a.UsedAt), cmp.Compare(a.ID, b.ID))
	}
	slices.SortFunc(first, byUse)
	slices.SortFunc(rest, byUse)
	return first, rest
}

// check checks entries, a few at once, within the budget, and records what
// it found in p. It reports false if the pass must stop, since GitHub
// can't be reached or a rate limit refused a request.
func (r *Revalidator) check(ctx context.Context, entries []Entry, p *Pass) bool {
	if len(entries) == 0 {
		return true
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	var (
		mu     sync.Mutex
		done   int
		paused bool
		wg     sync.WaitGroup
	)
	next := make(chan Entry)
	for range min(r.cfg.concurrency, len(entries)) {
		wg.Go(func() {
			for e := range next {
				at, ok := r.budget.wait(ctx, r.limit, func() { r.flushLocked(&mu, p) })
				if !ok {
					return
				}
				res := e.Check(ctx)
				if res.Status == Skipped {
					r.budget.release(at)
				}
				logCheck(ctx, e, res)
				mu.Lock()
				// A check that stopped because another paused the pass
				// found nothing.
				if ctx.Err() == nil || res.Status != Failed {
					done++
					r.record(p, e, res)
				}
				if res.Status == Offline || res.Status == Limited {
					paused = true
					stop()
				}
				mu.Unlock()
			}
		})
	}
feed:
	for _, e := range entries {
		select {
		case next <- e:
		case <-ctx.Done():
			break feed
		}
	}
	close(next)
	wg.Wait()
	p.Deferred += len(entries) - done
	r.flush(p)
	return !paused
}

// record adds the result of checking e to p. The caller holds the lock
// that guards p.
func (r *Revalidator) record(p *Pass, e Entry, res Result) {
	if res.Status != Skipped {
		p.Sent++
	}
	switch res.Status {
	case Skipped:
		p.Skipped++
	case NotModified:
		p.NotModified++
	case Changed:
		p.Changed++
	case Gone:
		p.Gone++
	case Offline:
		p.Offline = true
	case Limited:
		if res.RetryAt.After(p.RetryAt) {
			p.RetryAt = res.RetryAt
		}
	case Failed:
		p.Failed++
	}
	if res.Status == NotModified || res.Status == Changed {
		r.mu.Lock()
		r.checked[e.ID] = time.Now()
		if res.Status == Changed && res.Sync != "" {
			r.pending[res.Sync] = true
		}
		r.mu.Unlock()
	}
}

// flushLocked is flush under mu, the lock that guards p.
func (r *Revalidator) flushLocked(mu *sync.Mutex, p *Pass) {
	mu.Lock()
	defer mu.Unlock()
	r.flush(p)
}

// flush publishes the pending changes, each key once, and records them in
// p. The caller holds the lock that guards p, if any.
func (r *Revalidator) flush(p *Pass) {
	r.mu.Lock()
	keys := slices.Sorted(maps.Keys(r.pending))
	clear(r.pending)
	r.mu.Unlock()
	for _, k := range keys {
		r.cfg.publish(k)
	}
	p.Published = append(p.Published, keys...)
}

// logCheck logs what checking e found, at debug level.
func logCheck(ctx context.Context, e Entry, res Result) {
	if !obs.Enabled(ctx, slog.LevelDebug) {
		return
	}
	attrs := []slog.Attr{
		slog.String("span", "revalidate.check"),
		slog.String("entry", obs.LogKey(e.ID)),
		slog.String("status", res.Status.String()),
	}
	if e.Repo != (core.RepoRef{}) {
		attrs = append(attrs, slog.String("repo", e.Repo.String()))
	}
	if res.Err != nil {
		attrs = append(attrs, slog.String("err", res.Err.Error()))
	}
	slog.LogAttrs(ctx, slog.LevelDebug, "revalidate check", attrs...)
}

// log logs what p did, and when the next pass starts, at info level, or at
// warn level if GitHub couldn't be reached or a rate limit stopped it.
func (p *Pass) log(ctx context.Context, next time.Duration) {
	level := slog.LevelInfo
	if p.Offline || !p.RetryAt.IsZero() {
		level = slog.LevelWarn
	}
	if !obs.Enabled(ctx, level) {
		return
	}
	attrs := []slog.Attr{
		slog.String("span", "revalidate.pass"),
		slog.Float64("duration_ms", obs.Millis(p.End.Sub(p.Start))),
		slog.Int("listed", p.Listed),
		slog.Int("due", p.Due),
		slog.Int("budget", p.Budget),
		slog.Int("sent", p.Sent),
		slog.Int("not_modified", p.NotModified),
		slog.Int("changed", p.Changed),
		slog.Int("gone", p.Gone),
		slog.Int("failed", p.Failed),
		slog.Int("skipped", p.Skipped),
		slog.Int("deferred", p.Deferred),
		slog.Bool("offline", p.Offline),
		slog.Int("published", len(p.Published)),
		slog.Float64("next_in_s", next.Round(time.Second).Seconds()),
	}
	if !p.RetryAt.IsZero() {
		attrs = append(attrs, slog.Time("retry_at", p.RetryAt))
	}
	slog.LogAttrs(ctx, level, "revalidate pass", attrs...)
}
