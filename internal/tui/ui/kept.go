package ui

import (
	"context"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// FeedPages adapts a read of pages to a feed: query says which page a
// cursor selects, and read reads it. A page kept from an earlier session is
// shown while it is read again, unless it was served because GitHub
// couldn't be reached or rate limited the read, which the status bar
// tells. Each read is a trace named name, such as list.pulls.
//
// read gets again set on the read that follows a kept page of the same
// query, which must ask GitHub; any other read may still be served the kept
// page, such as another view's reading the same page at once.
func FeedPages[Q comparable, T any](name string, query func(cursor string) Q, read func(ctx context.Context, q Q, again bool) (core.Page[T], error)) feed.Fetch[T] {
	// kept holds the queries of the pages that came back kept, for the
	// feed to read again.
	var kept sync.Map
	return func(ctx context.Context, cursor string) ([]T, string, error) {
		q := query(cursor)
		_, again := kept.LoadAndDelete(q)
		ctx, end := obs.Begin(ctx, name)
		p, err := read(ctx, q, again)
		end(err, "span", "tui", "first", cursor == "", "items", len(p.Items), "stale", p.Stale, "offline", p.Offline, "limited", p.Limited)
		switch {
		case err != nil:
			return nil, "", err
		case p.Offline, p.Limited:
			// Read again now, it would be served the same.
		case p.Stale:
			kept.Store(q, true)
			return p.Items, p.Next, feed.ErrStale
		}
		return p.Items, p.Next, nil
	}
}
