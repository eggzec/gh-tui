// Package probe turns cheap change probes, conditional requests that GitHub
// answers with a free 304 when nothing changed, into watch.PollFuncs.
package probe

import (
	"context"
	"sync"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/watch"
)

// Func asks GitHub whether a resource changed since the response cond came
// from.
type Func func(ctx context.Context, cond github.Conditional) (github.Response, error)

// Tracker remembers the latest ETag of each probed resource. The zero value
// is ready to use, and it is safe for concurrent use.
type Tracker struct {
	mu    sync.Mutex
	etags map[string]string
	// kept holds the ETags across sessions, if Keep gave it a store.
	kept *cache.Shelf[string]
}

// kind is what a Tracker keeps its ETags as, and schema their version.
const (
	kind   = "probe"
	schema = 1
)

// Keep keeps the ETags in store too, so that the first probe of a session
// asks with the ETag the last session saw, and reports a change made in
// between. Call it before the first Poll. A nil store keeps nothing.
func (t *Tracker) Keep(store cache.Store) {
	t.kept = cache.NewShelf[string](store, kind, schema)
}

// Poll returns a watch.PollFunc that probes the resource named key with
// probe. The first probe of a key only records its ETag, since there is
// nothing to compare it with, and reports no change, unless an earlier
// session kept one. A later probe that brings a new ETag calls changed, so
// that the stale data is invalidated before the change is reported.
// Interval is the probe's X-Poll-Interval.
func (t *Tracker) Poll(key string, probe Func, changed func()) watch.PollFunc {
	return func(ctx context.Context) (watch.Result, error) {
		cond := github.Conditional{ETag: t.etag(key)}
		res, err := probe(ctx, cond)
		if err != nil {
			return watch.Result{}, err
		}
		out := watch.Result{Interval: res.PollInterval}
		if res.NotModified || res.ETag == "" {
			return out, nil
		}
		if t.record(key, res.ETag) {
			changed()
			out.Changed = true
		}
		return out, nil
	}
}

// ETag returns the latest ETag that a probe of key brought, this session
// or an earlier one, or "" if none did. What a read of the resource brings
// after this call is at least as recent as the ETag, so a 304 to it later
// vouches for that read. It may read the store, so call it where I/O is
// fine.
func (t *Tracker) ETag(key string) string {
	return t.etag(key)
}

// etag returns the latest ETag of key: this session's, or else the one an
// earlier session kept, which it then remembers.
func (t *Tracker) etag(key string) string {
	t.mu.Lock()
	etag, ok := t.etags[key]
	t.mu.Unlock()
	if ok {
		return etag
	}
	e, ok := t.kept.Load(key)
	if !ok || e.Value == "" {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if cur, ok := t.etags[key]; ok {
		return cur
	}
	t.remember(key, e.Value)
	return e.Value
}

// record stores etag under key and reports whether it replaced another one.
func (t *Tracker) record(key, etag string) bool {
	t.mu.Lock()
	old, seen := t.etags[key]
	t.remember(key, etag)
	t.mu.Unlock()
	if old != etag {
		// The store is only a shortcut, so a failure is ignored.
		_ = t.kept.Save(key, cache.Entry[string]{Value: etag})
	}
	return seen && old != etag
}

// remember stores etag under key. t.mu must be held.
func (t *Tracker) remember(key, etag string) {
	if t.etags == nil {
		t.etags = make(map[string]string)
	}
	t.etags[key] = etag
}
