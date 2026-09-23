package notifications

import (
	"context"
	"fmt"
	"slices"
	"time"

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

// MarkAllRead marks every thread updated until now as read in every cached
// page and returns the Op that tells GitHub. Threads that arrive later stay
// unread. GitHub may mark many threads in the background, so a refetch soon
// after may still show some of them unread.
func (s *Service) MarkAllRead() *optimistic.Op {
	// GitHub's timestamps have second precision.
	at := s.now().Truncate(time.Second)
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
// refresh.
func (s *Service) apply(ch change, send func(ctx context.Context) error) *optimistic.Op {
	rollback := s.cache.MutateTag(tag, ch)
	return optimistic.New(func(ctx context.Context) error {
		if err := send(ctx); err != nil {
			return err
		}
		s.cache.MutateTag(tag, ch)
		return nil
	}, rollback)
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
			if n := &p.Items[i]; !n.Unread || n.UpdatedAt.After(at) {
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
