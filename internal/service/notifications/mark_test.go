package notifications

import (
	"errors"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

var (
	now = time.Date(2026, 9, 23, 10, 0, 0, 500_000_000, time.UTC)

	// a and b are unread, b arrived after now; c was read.
	a = core.Notification{ID: "a", Unread: true, UpdatedAt: now.Add(-time.Hour)}
	b = core.Notification{ID: "b", Unread: true, UpdatedAt: now.Add(30 * time.Second)}
	c = core.Notification{ID: "c", UpdatedAt: now.Add(-2 * time.Hour)}

	readA = core.Notification{ID: "a", UpdatedAt: a.UpdatedAt}

	inbox    = ListQuery{}
	allInbox = ListQuery{Filter: core.NotificationFilter{All: true}}
)

// seeded returns a service whose cache holds the unread inbox [a b] and the
// full inbox [a c].
func seeded(api API) *Service {
	s := New(api)
	s.cache.Set(inbox.key(), entry(page{Items: []core.Notification{a, b}}, modified1))
	s.cache.Set(allInbox.key(), entry(page{Items: []core.Notification{a, c}, Next: "next"}, modified1))
	return s
}

var errFailed = errors.New("server said no")

type markCase struct {
	name string
	// fake makes the API answer the mark with err.
	fake func(api *fakeAPI, err error)
	mark func(s *Service) *optimistic.Op
	// want holds the pages after the change.
	want map[ListQuery]page
}

var markCases = []markCase{
	{
		name: "read",
		fake: func(api *fakeAPI, err error) {
			api.markRead = func(id string) error {
				if id != "a" {
					return errUnexpected
				}
				return err
			}
		},
		mark: func(s *Service) *optimistic.Op { return s.MarkRead("a") },
		want: map[ListQuery]page{
			inbox:    {Items: []core.Notification{readA, b}},
			allInbox: {Items: []core.Notification{readA, c}, Next: "next"},
		},
	},
	{
		name: "done",
		fake: func(api *fakeAPI, err error) {
			api.markDone = func(id string) error {
				if id != "a" {
					return errUnexpected
				}
				return err
			}
		},
		mark: func(s *Service) *optimistic.Op { return s.MarkDone("a") },
		want: map[ListQuery]page{
			inbox:    {Items: []core.Notification{b}},
			allInbox: {Items: []core.Notification{c}, Next: "next"},
		},
	},
	{
		name: "all read",
		fake: func(api *fakeAPI, err error) {
			api.markAll = func(at time.Time) error {
				if !at.Equal(a.UpdatedAt.Truncate(time.Second)) {
					return errUnexpected
				}
				return err
			}
		},
		// a is the newest thread seen: b is newer, so it stays unread.
		mark: func(s *Service) *optimistic.Op { return s.MarkAllRead(a.UpdatedAt) },
		want: map[ListQuery]page{
			inbox:    {Items: []core.Notification{readA, b}},
			allInbox: {Items: []core.Notification{readA, c}, Next: "next"},
		},
	},
}

func checkPages(t *testing.T, s *Service, want map[ListQuery]page) {
	t.Helper()
	for q, w := range want {
		if got, _ := s.CachedList(q); !equal(got, w) {
			t.Errorf("CachedList(%+v) = %+v, want %+v", q, got, w)
		}
	}
}

func snapshot(s *Service) map[ListQuery]page {
	pages := make(map[ListQuery]page)
	for _, q := range []ListQuery{inbox, allInbox} {
		pages[q], _ = s.CachedList(q)
	}
	return pages
}

func TestMarkSucceeds(t *testing.T) {
	for _, tt := range markCases {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{}
			tt.fake(api, nil)
			s := seeded(api)
			before := snapshot(s)

			op := tt.mark(s)
			checkPages(t, s, tt.want)
			if err := op.Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			checkPages(t, s, tt.want)
			if st := state(s, inbox); st != cache.Fresh {
				t.Errorf("inbox is %v after the change, want fresh", st)
			}
			// The change copied the pages rather than editing them.
			for q, p := range snapshot(seeded(&fakeAPI{})) {
				if !equal(before[q], p) {
					t.Errorf("page %+v was edited in place: %+v", q, before[q])
				}
			}
		})
	}
}

func TestMarkFails(t *testing.T) {
	for _, tt := range markCases {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{}
			tt.fake(api, errFailed)
			s := seeded(api)
			before := snapshot(s)

			op := tt.mark(s)
			checkPages(t, s, tt.want)
			if err := op.Do(t.Context()); !errors.Is(err, errFailed) {
				t.Fatalf("Do = %v, want errFailed", err)
			}
			checkPages(t, s, before)
			if st := state(s, inbox); st != cache.Fresh {
				t.Errorf("inbox is %v after rollback, want fresh as before", st)
			}
		})
	}
}

// A refetch that lands before GitHub made the change must not undo it once
// GitHub confirms.
func TestMarkReconciles(t *testing.T) {
	for _, tt := range markCases {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{}
			tt.fake(api, nil)
			s := seeded(api)
			api.onMark = func() {
				for q, p := range snapshot(seeded(&fakeAPI{})) {
					s.cache.Set(q.key(), entry(p, modified2))
				}
			}

			if err := tt.mark(s).Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			checkPages(t, s, tt.want)
		})
	}
}

func TestMarkUncached(t *testing.T) {
	for _, tt := range markCases {
		t.Run(tt.name, func(t *testing.T) {
			sent := false
			api := &fakeAPI{onMark: func() { sent = true }}
			tt.fake(api, nil)
			s := New(api)

			if err := tt.mark(s).Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if !sent {
				t.Error("the change was not sent")
			}
			if s.cache.Len() != 0 {
				t.Errorf("cache holds %d entries, want none", s.cache.Len())
			}
		})
	}
}

func state(s *Service, q ListQuery) cache.State {
	_, st := s.cache.Get(q.key())
	return st
}
