// Package notifications is the section of the user's inbox. It lists the
// threads, opens what each is about in the app through a threads.Opener,
// which reads them ahead too, and marks them read or done.
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
	"github.com/eggzec/gh-tui/internal/tui/threads"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// SyncKey is the sync key under which the app subscribes the service's Poll.
const SyncKey = notifications.SyncKey

// Service is what the section needs of the notifications service.
type Service interface {
	CachedList(q notifications.ListQuery) (core.Page[core.Notification], bool)
	List(ctx context.Context, q notifications.ListQuery) (core.Page[core.Notification], error)
	MarkRead(id string) *optimistic.Op
	MarkDone(id string) *optimistic.Op
	MarkAllRead(until time.Time) *optimistic.Op
	// Invalidate marks every cached page stale, so that the reads after it
	// ask GitHub.
	Invalidate()
}

// Option configures a Section.
type Option func(*Section)

// WithNow sets the clock that ages are measured against. The default is
// time.Now.
func WithNow(now func() time.Time) Option {
	return func(s *Section) { s.now = now }
}

// WithOffline shares off with other sections, so that the user is told once
// for all of them that GitHub can't be reached. By default the section has
// its own.
func WithOffline(off *ui.Offline) Option {
	return func(s *Section) {
		if off != nil {
			s.offline = off
		}
	}
}

// WithVoice sets how the section words what went wrong, with the keys a
// hint names and the log it points to. By default the hints name the
// configured keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(s *Section) { s.voice = v }
}

// WithOpener opens the threads, and reads them ahead while the section is
// on view, with o, which the dashboard may share. By default the section
// has one that reads nothing ahead and marks a thread read as it opens it.
func WithOpener(o *threads.Opener) Option {
	return func(s *Section) {
		if o != nil {
			s.opener = o
		}
	}
}

// Section lists the user's notifications. Create one with New.
type Section struct {
	ctx  context.Context
	svc  Service
	keys KeyMap
	now  func() time.Time

	feed feed.Model[core.Notification]
	// filt is the filter in force, nil for the default inbox. Fetches,
	// which run in commands, read it.
	filt    atomic.Pointer[filter]
	started bool
	// offline is marked by the feed's reads when GitHub can't be reached.
	offline *ui.Offline
	// voice words the feed's errors.
	voice ui.Voice
	// opener opens the threads and reads them ahead.
	opener *threads.Opener

	width, height int
	styles        styles
	// links keeps the links of the rows, which are drawn on every frame.
	links  termtext.Links
	header string
}

var (
	_ ui.Badger     = (*Section)(nil)
	_ ui.Filterable = (*Section)(nil)
)

// New returns the section, which reads through svc and binds the actions
// in keys. ctx bounds every request it makes.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{ctx: ctx, svc: svc, keys: newKeyMap(keys), now: time.Now, offline: new(ui.Offline), voice: ui.NewVoice(keys, "")}
	for _, opt := range opts {
		opt(s)
	}
	if s.opener == nil {
		s.opener = threads.New(ctx)
	}
	if !s.opener.MarksRead() {
		s.keys.Select.SetHelp(s.keys.Select.Help().Key, "open")
	}
	s.feed = feed.New(ui.FeedPages("list.notifications", s.offline, s.query, s.list), s.render,
		feed.WithContext(ctx),
		feed.WithKey(func(n core.Notification) string { return n.ID }),
		feed.WithKeyMap(s.keys.feed),
		feed.WithErrorText(ui.ErrorText("load the notifications", "", s.voice)),
	)
	s.SetTheme(ui.NewTheme(defaultPalette(), true))
	return s
}

// list reads a page for the feed, and keeps the threads the filter does.
// A page the filter leaves empty still leads to the next, which the feed
// reads while its window has room.
func (s *Section) list(ctx context.Context, q notifications.ListQuery, again bool) (core.Page[core.Notification], error) {
	f := s.filter()
	q.Again = again
	p, err := s.svc.List(ctx, q)
	p.Items = f.apply(p.Items)
	return p, err
}

func (s *Section) query(cursor string) notifications.ListQuery {
	return s.filter().listQuery(cursor)
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
func (s *Section) All() bool { return s.filter().all }

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

// Blur makes the list ignore keys, and stops the reads ahead of the
// threads, as the section leaves the screen.
func (s *Section) Blur() {
	s.feed.Blur()
	s.opener.Stop()
}

// Help lists the keys of the section for the help line.
func (s *Section) Help() help.KeyMap { return ui.Hints{Layers: s.KeyLayers()} }

// KeyLayers implements ui.Keyed: the section's own keys, with the one
// that clears the filter only while there is one, and then the list's.
func (s *Section) KeyLayers() []keyhelp.Layer {
	k := s.keys
	k.ClearFilter.SetEnabled(k.ClearFilter.Enabled() && s.filtered())
	return []keyhelp.Layer{keyhelp.FromHelp(ui.NotificationsTitle, k, false), keyhelp.FromHelp("list", s.feed.KeyMap(), false)}
}
