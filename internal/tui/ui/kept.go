package ui

import (
	"context"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// OfflineText is the toast that Offline shows, and LimitedText the one it
// shows when only rate limits kept reads from GitHub.
const (
	OfflineText = "Can't reach GitHub, so this is what the last visit showed."
	LimitedText = "Rate limited by GitHub, so this is what the last visit showed."
)

// Offline tells the user once that what the sections show was read earlier,
// because GitHub couldn't be reached or rate limited the reads. Sections
// may share one, so that the user is told once for all of them. Reads call
// Mark or MarkLimited from their commands, and the sections' Update calls
// Notify. The zero value is ready to use.
type Offline struct {
	marked, limited atomic.Bool
	told            bool
}

// Mark records that a read was served offline. It is safe to call from a
// command.
func (o *Offline) Mark() {
	o.marked.Store(true)
}

// MarkLimited records that a read was served because GitHub rate limited
// it. It is safe to call from a command.
func (o *Offline) MarkLimited() {
	o.limited.Store(true)
}

// Notify returns a toast the first time it is called after a Mark or a
// MarkLimited, and nil otherwise: OfflineText if a read was served
// offline, and LimitedText if reads were only rate limited. Call it from
// Update.
func (o *Offline) Notify() tea.Cmd {
	offline, limited := o.marked.Load(), o.limited.Load()
	if o.told || !offline && !limited {
		return nil
	}
	o.told = true
	if !offline {
		return Notify(toast.Info, LimitedText)
	}
	return Notify(toast.Info, OfflineText)
}

// FeedPages adapts a read of pages to a feed: query says which page a
// cursor selects, and read reads it. A page kept from an earlier session is
// shown while it is read again, and a page served offline or limited is
// marked on off. Each read is a trace named name, such as list.pulls.
//
// read gets again set on the read that follows a kept page of the same
// query, which must ask GitHub; any other read may still be served the kept
// page, such as another view's reading the same page at once.
func FeedPages[Q comparable, T any](name string, off *Offline, query func(cursor string) Q, read func(ctx context.Context, q Q, again bool) (core.Page[T], error)) feed.Fetch[T] {
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
		case p.Offline:
			off.Mark()
		case p.Limited:
			off.MarkLimited()
		case p.Stale:
			kept.Store(q, true)
			return p.Items, p.Next, feed.ErrStale
		}
		return p.Items, p.Next, nil
	}
}
