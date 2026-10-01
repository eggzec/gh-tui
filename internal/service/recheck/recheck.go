// Package recheck adapts what the services keep on a cache.Shelf to a
// revalidate.Revalidator: it lists the kept entries that a conditional
// request can revalidate, and turns what a check found into a
// revalidate.Result.
package recheck

import (
	"context"
	"errors"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

// Target is what a service knows of a kept entry from its key: the
// repository it belongs to, and how to check it.
type Target struct {
	Repo  core.RepoRef
	Check func(ctx context.Context) revalidate.Result
}

// Entries returns the entries that shelf keeps with validators, as
// entries of kind for a revalidator, each fresh for ttl, the TTL of the
// cache they are read into. target returns the target of the entry under
// a key, or false for a key it can't check.
func Entries[V any](shelf *cache.Shelf[V], kind string, ttl time.Duration, target func(key string) (Target, bool)) []revalidate.Entry {
	var out []revalidate.Entry
	for k := range shelf.Kept() {
		if k.ETag == "" && k.LastModified == "" {
			continue
		}
		t, ok := target(k.Key)
		if !ok {
			continue
		}
		out = append(out, revalidate.Entry{
			ID:        kind + ":" + k.Key,
			Repo:      t.Repo,
			UsedAt:    k.UsedAt,
			CheckedAt: k.FetchedAt,
			FreshFor:  ttl,
			Check:     t.Check,
		})
	}
	return out
}

// Check rechecks the entry under key with fn, in c if c holds it and
// otherwise on shelf, as cache.Shelf.Recheck does, and returns it with the
// result. A changed entry reports sync. What GitHub refuses is dropped from
// shelf, as a read would.
func Check[V any](ctx context.Context, c *cache.Cache[V], shelf *cache.Shelf[V], key, sync string, fn cache.FetchFunc[V]) (cache.Entry[V], revalidate.Result) {
	e, found, err := shelf.Recheck(ctx, c, key, fn)
	if err == nil {
		switch found {
		case cache.RecheckNotModified:
			return e, revalidate.Result{Status: revalidate.NotModified}
		case cache.RecheckChanged:
			return e, revalidate.Result{Status: revalidate.Changed, Sync: sync}
		default:
			return e, revalidate.Result{Status: revalidate.Skipped}
		}
	}
	res := Failure(ctx, err)
	if res.Status == revalidate.Gone {
		shelf.Delete(key)
	}
	return e, res
}

// Failure is the result of a check that failed with err.
func Failure(ctx context.Context, err error) revalidate.Result {
	if rl, ok := errors.AsType[*core.RateLimitError](err); ok {
		return revalidate.Result{Status: revalidate.Limited, RetryAt: rl.Reset, Err: err}
	}
	switch {
	case github.Unreachable(ctx, err):
		return revalidate.Result{Status: revalidate.Offline, Err: err}
	case github.Refused(err):
		return revalidate.Result{Status: revalidate.Gone, Err: err}
	default:
		return revalidate.Result{Status: revalidate.Failed, Err: err}
	}
}

// Load adapts a conditional read from GitHub to a cache.FetchFunc, whose
// entries carry the response's validators and URL and the tags that tags
// returns for the value.
func Load[V any](get func(ctx context.Context, cond github.Conditional) (V, github.Response, error), tags func(V) []string) cache.FetchFunc[V] {
	return func(ctx context.Context, prev cache.Entry[V], ok bool) (cache.Entry[V], error) {
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		v, res, err := get(ctx, cond)
		switch {
		case err != nil:
			return cache.Entry[V]{}, err
		case res.NotModified:
			return cache.Entry[V]{}, cache.ErrNotModified
		}
		return cache.Entry[V]{Value: v, ETag: res.ETag, LastModified: res.LastModified, Source: res.URL, Tags: tags(v)}, nil
	}
}
