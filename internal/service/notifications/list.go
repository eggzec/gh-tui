package notifications

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/fallback"
	"github.com/eggzec/gh-tui/internal/watch"
)

// tag marks every cached page, so a change to a thread can find each page
// that lists it.
const tag = "notifications"

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
	// PageSize is how many threads a page holds. Zero means the service's
	// page size, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

// normalize returns q with its defaults set: size is the service's page
// size.
func (q ListQuery) normalize(size int) ListQuery {
	if q.PageSize <= 0 {
		q.PageSize = size
	}
	q.PageSize = min(q.PageSize, maxPageSize)
	return q
}

// key is the cache key of q, which the defaults and the clamp don't change;
// size is the service's page size.
func (q ListQuery) key(size int) string {
	q = q.normalize(size)
	return "notifications?all=" + strconv.FormatBool(q.Filter.All) +
		"&participating=" + strconv.FormatBool(q.Filter.Participating) +
		"&page_size=" + strconv.Itoa(q.PageSize) +
		"&cursor=" + q.Cursor
}

// CachedList returns the cached page for q, fresh or stale, without a
// request. It reports false if the page isn't cached, or if the token may
// not read notifications: what another token read isn't this one's to see.
func (s *Service) CachedList(q ListQuery) (core.Page[core.Notification], bool) {
	if s.refused() != nil {
		return page{}, false
	}
	e, st := s.cache.Get(q.key(s.pageSize))
	return e.Value, st != cache.Miss
}

// List returns the page for q. A fresh cached page is returned as is; a stale
// one is revalidated with its validators, which costs no rate limit when
// nothing changed.
//
// A page that only an earlier session kept is fresh if it was fetched or
// revalidated within the TTL. An older one is returned at once, with Stale
// set, to every read until one with q.Again set revalidates it. If GitHub
// can't be reached, a stale page is served with Offline set, and if it
// rate limits the read, with Limited set.
//
// When the token may not read notifications, List returns why at once,
// and serves nothing it holds.
func (s *Service) List(ctx context.Context, q ListQuery) (core.Page[core.Notification], error) {
	if err := s.refused(); err != nil {
		return page{}, fmt.Errorf("list notifications: %w", err)
	}
	q = q.normalize(s.pageSize)
	if e, ok := s.kept.Warm(s.cache, q.key(s.pageSize), q.Again); ok {
		p := e.Value
		p.Stale = true
		return p, nil
	}
	// The client already names the request in its error.
	e, err := fallback.Fetch(ctx, s.cache, s.kept, q.key(s.pageSize), fallback.Page[core.Notification], s.load(q))
	return e.Value, err
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
// zero ListQuery afterwards is a fresh hit. Interval is GitHub's
// X-Poll-Interval. A poll that GitHub didn't answer fails, even though
// the page read last is served to the reads meanwhile.
//
// While the token may not read notifications, Poll asks nothing and
// reports no change, so polling pauses without failing, and resumes on
// the first poll after the token may.
func (s *Service) Poll(ctx context.Context) (watch.Result, error) {
	if s.refused() != nil {
		return watch.Result{}, nil
	}
	q := ListQuery{}.normalize(s.pageSize)
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
	s.kept.Warm(s.cache, q.key(s.pageSize), true)
	// A fresh entry would be returned without a request, and polling must
	// ask the server. If a List is already revalidating, Poll joins it and
	// reports no change: that List hands its caller the new data.
	s.cache.Invalidate(q.key(s.pageSize))
	e, err := fallback.Fetch(ctx, s.cache, s.kept, q.key(s.pageSize), fallback.Page[core.Notification], fn)
	if err == nil {
		err = e.Fallback
	}
	if err != nil {
		return watch.Result{}, fmt.Errorf("poll notifications: %w", err)
	}
	return watch.Result{Changed: changed, Interval: time.Duration(s.interval.Load())}, nil
}

// load fetches the page for a normalized q, conditionally when a previous
// entry exists, and keeps what GitHub sends.
func (s *Service) load(q ListQuery) cache.FetchFunc[page] {
	return fallback.Keep(s.kept, q.key(s.pageSize), s.fetcher(q))
}
