package notifications

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/watch"
)

// tag marks every cached page, so a change to a thread can find each page
// that lists it.
const tag = "notifications"

// DefaultPageSize is the page size of a ListQuery that sets none.
const DefaultPageSize = 30

// maxPageSize is the largest page GitHub returns.
const maxPageSize = 100

// ListQuery selects a page of notifications. The zero ListQuery is the first
// page of the default inbox, which Poll watches.
//
// The Next of a page is a URL that already carries the filter and the page
// size, so when Cursor is set, the page has the Filter and PageSize of the
// first page, whatever the query says. Both still key the cache, so a query
// should repeat them to find the page again.
type ListQuery struct {
	Filter core.NotificationFilter
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many threads a page holds. Zero means DefaultPageSize,
	// and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

func (q ListQuery) normalize() ListQuery {
	if q.PageSize <= 0 {
		q.PageSize = DefaultPageSize
	}
	q.PageSize = min(q.PageSize, maxPageSize)
	return q
}

// key is the cache key of q, which the defaults and the clamp don't change.
func (q ListQuery) key() string {
	q = q.normalize()
	return "notifications?all=" + strconv.FormatBool(q.Filter.All) +
		"&participating=" + strconv.FormatBool(q.Filter.Participating) +
		"&page_size=" + strconv.Itoa(q.PageSize) +
		"&cursor=" + q.Cursor
}

// CachedList returns the cached page for q, fresh or stale, without a
// request. It reports false if the page isn't cached.
func (s *Service) CachedList(q ListQuery) (core.Page[core.Notification], bool) {
	e, st := s.cache.Get(q.key())
	return e.Value, st != cache.Miss
}

// List returns the page for q. A fresh cached page is returned as is; a stale
// one is revalidated with its validators, which costs no rate limit when
// nothing changed.
//
// A page that only an earlier session kept is fresh if it was fetched or
// revalidated within the TTL. An older one is returned at once, with Stale
// set, to every read until one with q.Again set revalidates it. If GitHub
// can't be reached, a stale page is served with Offline set.
func (s *Service) List(ctx context.Context, q ListQuery) (core.Page[core.Notification], error) {
	q = q.normalize()
	if e, ok := s.kept.Warm(s.cache, q.key(), q.Again); ok {
		p := e.Value
		p.Stale = true
		return p, nil
	}
	e, err := s.cache.Fetch(ctx, q.key(), s.load(q))
	if err != nil {
		return core.Page[core.Notification]{}, fmt.Errorf("list notifications: %w", err)
	}
	return e.Value, nil
}

// Invalidate marks every cached page stale. The pages are still served by
// CachedList, and the next List of each revalidates it with its validators,
// so a refresh reaches the server even while the pages are fresh, and costs
// no rate limit if nothing changed.
func (s *Service) Invalidate() {
	s.cache.InvalidateTag(tag)
}

// Poll revalidates the default inbox, as a watch.PollFunc. Changed is true
// only when GitHub sent new data, which is then cached, so refetching the
// zero ListQuery afterwards is a fresh hit. Interval is GitHub's X-Poll-Interval.
func (s *Service) Poll(ctx context.Context) (watch.Result, error) {
	q := ListQuery{}.normalize()
	load := s.load(q)
	// Fetch returns after fn unless ctx is done, and then changed is not
	// read, so it needs no lock.
	var changed bool
	fn := func(ctx context.Context, prev cache.Entry[page], ok bool) (cache.Entry[page], error) {
		e, err := load(ctx, prev, ok)
		changed = err == nil
		return e, err
	}
	// What an earlier session kept makes the request conditional.
	s.kept.Warm(s.cache, q.key(), true)
	// A fresh entry would be returned without a request, and polling must
	// ask the server. If a List is already revalidating, Poll joins it and
	// reports no change: that List hands its caller the new data.
	s.cache.Invalidate(q.key())
	if _, err := s.cache.Fetch(ctx, q.key(), fn); err != nil {
		return watch.Result{}, fmt.Errorf("poll notifications: %w", err)
	}
	return watch.Result{Changed: changed, Interval: time.Duration(s.interval.Load())}, nil
}

// offlineAt is when a page served offline was fetched, as far as the cache
// can tell: long ago, so it is stale at once and the next read asks GitHub
// again.
var offlineAt = time.Unix(1, 0)

// load fetches the page for a normalized q, conditionally when a previous
// entry exists, and keeps what GitHub sends. If GitHub can't be reached,
// the previous entry is served with Offline set.
func (s *Service) load(q ListQuery) cache.FetchFunc[page] {
	key := q.key()
	return func(ctx context.Context, prev cache.Entry[page], ok bool) (cache.Entry[page], error) {
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		p, res, err := s.api.ListNotifications(ctx, q.Filter, q.PageSize, q.Cursor, cond)
		switch {
		case ok && github.Unreachable(ctx, err):
			prev.Value.Offline, prev.FetchedAt = true, offlineAt
			return prev, nil
		case err != nil:
			if github.Refused(err) {
				s.kept.Delete(key)
			}
			return cache.Entry[page]{}, err
		}
		if res.PollInterval > 0 {
			s.interval.Store(int64(res.PollInterval))
		}
		if res.NotModified {
			return cache.Entry[page]{}, cache.ErrNotModified
		}
		e := cache.Entry[page]{
			Value:        p,
			ETag:         res.ETag,
			LastModified: res.LastModified,
			Source:       res.URL,
			Tags:         []string{tag},
		}
		// The shelf is only a shortcut, so a failure is ignored.
		_ = s.kept.Save(key, e)
		return e, nil
	}
}
