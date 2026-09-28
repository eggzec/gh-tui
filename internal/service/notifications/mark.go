package notifications

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// A change edits a cached page. It reports whether it changed anything and
// returns a copy rather than editing the page, which rollback restores.
type change func(page) (page, bool)

// MarkRead marks thread id as read in every cached page and returns the Op
// that tells GitHub. Pages of unread threads keep the thread, now read, until
// they are refetched, so the list doesn't shift under the cursor.
func (s *Service) MarkRead(id string) *optimistic.Op {
	return s.apply(markRead(id), func(ctx context.Context) error {
		if err := s.api.MarkThreadRead(ctx, id); err != nil {
			return fmt.Errorf("mark notification %s read: %w", id, err)
		}
		return nil
	})
}

// MarkDone removes thread id from every cached page and returns the Op that
// tells GitHub.
func (s *Service) MarkDone(id string) *optimistic.Op {
	return s.apply(remove(id), func(ctx context.Context) error {
		if err := s.api.MarkThreadDone(ctx, id); err != nil {
			return fmt.Errorf("mark notification %s done: %w", id, err)
		}
		return nil
	})
}

// MarkAllRead marks every thread updated until until as read in every
// cached page and returns the Op that tells GitHub. Threads updated after
// it stay unread, so passing the newest thread the user saw leaves those
// they didn't see unread. GitHub may mark many threads in the background,
// so a refetch soon after may still show some of them unread.
func (s *Service) MarkAllRead(until time.Time) *optimistic.Op {
	// GitHub's timestamps have second precision.
	at := until.Truncate(time.Second)
	return s.apply(markReadUntil(at), func(ctx context.Context) error {
		if err := s.api.MarkNotificationsRead(ctx, at); err != nil {
			return fmt.Errorf("mark all notifications read: %w", err)
		}
		return nil
	})
}

// apply makes ch to the cache now and returns an Op that sends it. GitHub
// answers these requests without the thread, so on success ch is applied
// again rather than storing a response: a page that was refetched before
// GitHub made the change would otherwise show the old state until the next
// refresh. When the token may not mark notifications, nothing changes and
// the Op returns why.
func (s *Service) apply(ch change, send func(ctx context.Context) error) *optimistic.Op {
	if err := s.refused(); err != nil {
		return optimistic.Refused(fmt.Errorf("mark notifications: %w", err))
	}
	rollback := s.cache.MutateTag(tag, ch)
	return optimistic.New(func(ctx context.Context) error {
		if err := send(ctx); err != nil {
			return err
		}
		s.cache.MutateTag(tag, ch)
		s.keep()
		return nil
	}, rollback)
}

// keep saves the pages as they are in memory now that GitHub confirmed a
// change, so the next session doesn't start from before it. Until then
// only memory has the change, so a rollback leaves nothing behind. The
// validators are left out: they describe the pages before the change, and a
// 304 to them must not bring back what the pages were before.
func (s *Service) keep() {
	for key, e := range s.cache.TaggedEntries(tag) {
		p := e.Value
		p.Offline = false
		_ = s.kept.Save(key, cache.Entry[page]{Value: p, FetchedAt: e.FetchedAt, Tags: e.Tags})
	}
}

func markRead(id string) change {
	return func(p page) (page, bool) {
		i := slices.IndexFunc(p.Items, func(n core.Notification) bool { return n.ID == id })
		if i < 0 || !p.Items[i].Unread {
			return p, false
		}
		p.Items = slices.Clone(p.Items)
		p.Items[i].Unread = false
		return p, true
	}
}

func markReadUntil(at time.Time) change {
	return func(p page) (page, bool) {
		changed := false
		for i := range p.Items {
			// GitHub compares at the second, as at is.
			if n := &p.Items[i]; !n.Unread || n.UpdatedAt.Truncate(time.Second).After(at) {
				continue
			}
			if !changed {
				p.Items, changed = slices.Clone(p.Items), true
			}
			p.Items[i].Unread = false
		}
		return p, changed
	}
}

func remove(id string) change {
	return func(p page) (page, bool) {
		i := slices.IndexFunc(p.Items, func(n core.Notification) bool { return n.ID == id })
		if i < 0 {
			return p, false
		}
		p.Items = slices.Delete(slices.Clone(p.Items), i, i+1)
		return p, true
	}
}
