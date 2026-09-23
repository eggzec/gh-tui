// Package notifications is the section of the user's inbox. Threads open in
// the browser; the section lists them and marks them read or done.
package notifications

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// SyncKey is the sync key under which the app subscribes the service's Poll.
const SyncKey = "notifications"

// Service is what the section needs of the notifications service.
type Service interface {
	CachedList(q notifications.ListQuery) (core.Page[core.Notification], bool)
	List(ctx context.Context, q notifications.ListQuery) (core.Page[core.Notification], error)
	MarkRead(id string) *optimistic.Op
	MarkDone(id string) *optimistic.Op
	MarkAllRead() *optimistic.Op
}

// Option configures a Section.
type Option func(*Section)

// WithNow sets the clock that ages are measured against. The default is
// time.Now.
func WithNow(now func() time.Time) Option {
	return func(s *Section) { s.now = now }
}

// Section lists the user's notifications. Create one with New.
type Section struct {
	ctx  context.Context
	svc  Service
	keys KeyMap
	now  func() time.Time

	feed feed.Model[core.Notification]
	// all is read by fetches, which run in commands.
	all     atomic.Bool
	started bool

	width, height int
	styles        styles
	header        string
}

var _ ui.Badger = (*Section)(nil)

// New returns the section, which reads through svc and binds the actions
// in keys. ctx bounds every request it makes.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{ctx: ctx, svc: svc, keys: newKeyMap(keys), now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	s.feed = feed.New(s.fetch, s.render,
		feed.WithContext(ctx),
		feed.WithKey(func(n core.Notification) string { return n.ID }),
		feed.WithKeyMap(s.keys.feed),
	)
	s.SetTheme(ui.NewTheme(defaultPalette(), true))
	return s
}

// fetch adapts the service's List to the feed.
func (s *Section) fetch(ctx context.Context, cursor string) ([]core.Notification, string, error) {
	p, err := s.svc.List(ctx, s.query(cursor))
	if err != nil {
		return nil, "", err
	}
	return p.Items, p.Next, nil
}

func (s *Section) query(cursor string) notifications.ListQuery {
	return notifications.ListQuery{
		Filter: core.NotificationFilter{All: s.all.Load()},
		Cursor: cursor,
	}
}

// Title returns the title of the tab.
func (s *Section) Title() string { return ui.NotificationsTitle }

// Init fetches the first page.
func (s *Section) Init() tea.Cmd {
	s.started = true
	return s.feed.Init()
}

// Badge returns the number of unread threads in the cached first page of
// the default inbox, with a "+" when more pages follow, or "" when there are
// none. It does no I/O.
func (s *Section) Badge() string {
	p, ok := s.svc.CachedList(notifications.ListQuery{})
	if !ok {
		return ""
	}
	n := 0
	for i := range p.Items {
		if p.Items[i].Unread {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	b := strconv.Itoa(n)
	if !p.Last() {
		b += "+"
	}
	return b
}

// All reports whether the section shows read threads too.
func (s *Section) All() bool { return s.all.Load() }

// SetSize sets the size of the section. The first line shows the filter.
func (s *Section) SetSize(width, height int) {
	s.width, s.height = width, height
	s.feed.SetSize(width, max(height-headerHeight, 0))
	s.renderHeader()
}

// SetTheme builds the styles of the rows and restyles the list.
func (s *Section) SetTheme(t ui.Theme) {
	s.styles = newStyles(t)
	s.feed.SetStyles(t.Feed())
	s.renderHeader()
}

// Focus makes the list react to keys.
func (s *Section) Focus() { s.feed.Focus() }

// Blur makes the list ignore keys.
func (s *Section) Blur() { s.feed.Blur() }

// Help returns the keys of the section, the list's navigation included.
func (s *Section) Help() help.KeyMap { return s.keys.withFeed(s.feed.KeyMap()) }
