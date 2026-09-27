// Package fallback reads what the services cache through their
// cache.Cache, falling back on what was read last while GitHub can't
// answer: when it can't be reached, fails on its side or rate limits the
// read, the previous entry is served in place of the answer, marked, and
// when it refuses, what a cache.Shelf keeps is dropped.
package fallback

import (
	"context"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// Marks returns the flags of v that say it was served in place of an
// answer: offline, because GitHub couldn't be reached or failed on its
// side, and limited, because it rate limited the read. A value without
// such flags is served unmarked; None is its Marks.
type Marks[V any] func(v *V) (offline, limited *bool)

// None is the Marks of a value that has no flags to mark.
func None[V any](*V) (offline, limited *bool) { return nil, nil }

// Page is the Marks of a page.
func Page[T any](p *core.Page[T]) (offline, limited *bool) { return &p.Offline, &p.Limited }

// staleAt is when an entry served in place of an answer was fetched, as
// far as the cache can tell: long ago, so it is stale at once and the next
// read asks GitHub again.
var staleAt = time.Unix(1, 0)

// Fetch returns the entry under key in c. A fresh one comes from memory;
// otherwise load reads it, with the previous entry's validators if it has
// one. load keeps what GitHub sends on shelf itself, since only it knows
// what is GitHub's, and reports cache.ErrNotModified when GitHub confirms
// the previous entry.
//
// When GitHub can't be reached, fails on its side or rate limits the read,
// the previous entry is served, with marks set on its value, and stays
// stale, so the next read asks again. The cache holds it unmarked, with
// cache.Entry.Fallback set, so any answer, a 304 too, leaves it unmarked.
// When GitHub refuses the read, for the token or the account, or says
// that what was asked isn't there, what shelf keeps under key is dropped,
// since the account may have lost access.
func Fetch[V any](ctx context.Context, c *cache.Cache[V], shelf *cache.Shelf[V], key string, marks Marks[V], load cache.FetchFunc[V]) (cache.Entry[V], error) {
	e, err := c.Fetch(ctx, key, func(ctx context.Context, prev cache.Entry[V], ok bool) (cache.Entry[V], error) {
		e, err := load(ctx, prev, ok)
		if err == nil {
			e.Fallback = nil
			return e, nil
		}
		switch core.KindOf(err) {
		case core.Offline, core.Unavailable, core.RateLimited:
			// After ctx is done, what failed was canceled, not refused.
			if ok && ctx.Err() == nil {
				prev.Fallback, prev.FetchedAt = err, staleAt
				return prev, nil
			}
		case core.Auth, core.Forbidden, core.NotFound:
			shelf.Delete(key)
		default:
		}
		return cache.Entry[V]{}, err
	})
	if err != nil {
		return cache.Entry[V]{}, err
	}
	if e.Fallback != nil {
		mark(&e.Value, marks, e.Fallback)
	}
	return e, nil
}

// Keep returns load, which also keeps what it reads from GitHub on shelf
// under key. What it reports as not modified isn't kept again: the
// previous entry may hold changes that GitHub hasn't confirmed.
func Keep[V any](shelf *cache.Shelf[V], key string, load cache.FetchFunc[V]) cache.FetchFunc[V] {
	return func(ctx context.Context, prev cache.Entry[V], ok bool) (cache.Entry[V], error) {
		e, err := load(ctx, prev, ok)
		if err != nil {
			return e, err
		}
		// The shelf is only a shortcut, so a failure is ignored.
		_ = shelf.Save(key, e)
		return e, nil
	}
}

// mark sets the flag of v that says why it was served in place of the
// answer that failed with err.
func mark[V any](v *V, marks Marks[V], err error) {
	offline, limited := marks(v)
	flag := offline
	if core.KindOf(err) == core.RateLimited {
		flag = limited
	}
	if flag != nil {
		*flag = true
	}
}
