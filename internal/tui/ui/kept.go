package ui

import (
	"context"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// OfflineText is the toast that Offline shows.
const OfflineText = "Can't reach GitHub, so this is what the last visit showed."

// Offline tells the user once that what the sections show was read earlier,
// because GitHub couldn't be reached. Sections may share one, so that the
// user is told once for all of them. Reads call Mark from their commands,
// and the sections' Update calls Notify. The zero value is ready to use.
type Offline struct {
	marked atomic.Bool
	told   bool
}

// Mark records that a read was served offline. It is safe to call from a
// command.
func (o *Offline) Mark() {
	o.marked.Store(true)
}

// Notify returns a toast of OfflineText the first time it is called after
// a Mark, and nil otherwise. Call it from Update.
func (o *Offline) Notify() tea.Cmd {
	if o.told || !o.marked.Load() {
		return nil
	}
	o.told = true
	return Notify(toast.Info, OfflineText)
}

// FeedPages adapts a read of pages to a feed. A page kept from an earlier
// session is shown while it is read again, and a page served offline marks
// off.
func FeedPages[T any](off *Offline, read func(ctx context.Context, cursor string) (core.Page[T], error)) feed.Fetch[T] {
	return func(ctx context.Context, cursor string) ([]T, string, error) {
		p, err := read(ctx, cursor)
		switch {
		case err != nil:
			return nil, "", err
		case p.Offline:
			off.Mark()
		case p.Stale:
			return p.Items, p.Next, feed.ErrStale
		}
		return p.Items, p.Next, nil
	}
}
