// Package probe turns cheap change probes, conditional requests that GitHub
// answers with a free 304 when nothing changed, into watch.PollFuncs.
package probe

import (
	"context"
	"sync"

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
}

// Poll returns a watch.PollFunc that probes the resource named key with
// probe. The first probe of a key only records its ETag, since there is
// nothing to compare it with, and reports no change. A later probe that
// brings a new ETag calls changed, so that the stale data is invalidated
// before the change is reported. Interval is the probe's X-Poll-Interval.
func (t *Tracker) Poll(key string, probe Func, changed func()) watch.PollFunc {
	return func(ctx context.Context) (watch.Result, error) {
		t.mu.Lock()
		cond := github.Conditional{ETag: t.etags[key]}
		t.mu.Unlock()

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

// record stores etag under key and reports whether it replaced another one.
func (t *Tracker) record(key, etag string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	old, seen := t.etags[key]
	if t.etags == nil {
		t.etags = make(map[string]string)
	}
	t.etags[key] = etag
	return seen && old != etag
}
