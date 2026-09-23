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

// Query selects a page of notifications. The zero Query is the first page of
// the default inbox, which Poll watches.
type Query struct {
	Filter core.NotificationFilter
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
}

func (q Query) key() string {
	return "notifications?all=" + strconv.FormatBool(q.Filter.All) +
		"&participating=" + strconv.FormatBool(q.Filter.Participating) +
		"&cursor=" + q.Cursor
}

// Cached returns the cached page for q, fresh or stale, without a request.
// It reports false if the page isn't cached.
func (s *Service) Cached(q Query) (core.Page[core.Notification], bool) {
	e, st := s.cache.Get(q.key())
	return e.Value, st != cache.Miss
}

// List returns the page for q. A fresh cached page is returned as is; a stale
// one is revalidated with its validators, which costs no rate limit when
// nothing changed.
func (s *Service) List(ctx context.Context, q Query) (core.Page[core.Notification], error) {
	e, err := s.cache.Fetch(ctx, q.key(), s.load(q))
	if err != nil {
		return core.Page[core.Notification]{}, fmt.Errorf("list notifications: %w", err)
	}
	return e.Value, nil
}

// Poll revalidates the default inbox, as a watch.PollFunc. Changed is true
// only when GitHub sent new data, which is then cached, so refetching the
// zero Query afterwards is a fresh hit. Interval is GitHub's X-Poll-Interval.
func (s *Service) Poll(ctx context.Context) (watch.Result, error) {
	var q Query
	load := s.load(q)
	// Fetch returns after fn unless ctx is done, and then changed is not
	// read, so it needs no lock.
	var changed bool
	fn := func(ctx context.Context, prev cache.Entry[page], ok bool) (cache.Entry[page], error) {
		e, err := load(ctx, prev, ok)
		changed = err == nil
		return e, err
	}
	// A fresh entry would be returned without a request, and polling must
	// ask the server. If a List is already revalidating, Poll joins it and
	// reports no change: that List hands its caller the new data.
	s.cache.Invalidate(q.key())
	if _, err := s.cache.Fetch(ctx, q.key(), fn); err != nil {
		return watch.Result{}, fmt.Errorf("poll notifications: %w", err)
	}
	return watch.Result{Changed: changed, Interval: time.Duration(s.interval.Load())}, nil
}

// load fetches the page for q, conditionally when a previous entry exists.
func (s *Service) load(q Query) cache.FetchFunc[page] {
	return func(ctx context.Context, prev cache.Entry[page], ok bool) (cache.Entry[page], error) {
		var cond github.Conditional
		if ok {
			cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
		}
		p, res, err := s.api.ListNotifications(ctx, q.Filter, q.Cursor, cond)
		if err != nil {
			return cache.Entry[page]{}, err
		}
		if res.PollInterval > 0 {
			s.interval.Store(int64(res.PollInterval))
		}
		if res.NotModified {
			return cache.Entry[page]{}, cache.ErrNotModified
		}
		return cache.Entry[page]{
			Value:        p,
			ETag:         res.ETag,
			LastModified: res.LastModified,
			Tags:         []string{tag},
		}, nil
	}
}
