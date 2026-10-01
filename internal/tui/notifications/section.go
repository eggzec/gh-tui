// Package notifications is the section of the user's inbox. It lists the
// threads, opens what each is about in the app through a threads.Opener,
// which reads them ahead too, and marks them read or done.
package notifications

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
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

// WithVoice sets how the section words what went wrong, with the keys a
// hint names and the log it points to. By default the hints name the
// configured keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(s *Section) { s.voice = v }
}

// WithIcons sets the icons whose error glyph marks a failed read. Without
// it, the icons are the config's default.
func WithIcons(ic ui.Icons) Option {
	return func(s *Section) { s.icons = ic }
}

// WithDates sets how dates read, as ui.date_format says. The default is
// as ages.
func WithDates(d ui.Dates) Option {
	return func(s *Section) { s.dates = d }
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
	// voice words the feed's errors, and icons mark them.
	voice ui.Voice
	icons ui.Icons
	// dates tell when the threads were updated.
	dates ui.Dates
	// opener opens the threads and reads them ahead.
	opener *threads.Opener

	width, height int
	theme         ui.Theme
	styles        styles
	// links keeps the links of the rows, which are drawn on every frame.
	links  termtext.Links
	header string
	// unreadable is drawn in place of the list while the token may not
	// read notifications, or is "".
	unreadable string
}

var (
	_ ui.Badger     = (*Section)(nil)
	_ ui.Filterable = (*Section)(nil)
)

// New returns the section, which reads through svc and binds the actions
// in keys. ctx bounds every request it makes.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{ctx: ctx, svc: svc, keys: newKeyMap(keys), now: time.Now, voice: ui.NewVoice(keys, ""), icons: ui.NewIcons(config.Default().UI.Icons)}
	for _, opt := range opts {
		opt(s)
	}
	if s.opener == nil {
		s.opener = threads.New(ctx)
	}
	if !s.opener.MarksRead() {
		s.keys.Select.SetHelp(s.keys.Select.Help().Key, "open")
	}
	s.feed = feed.New(ui.FeedPages("list.notifications", s.query, s.list), s.render,
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
// reads while its window has room. While the token may not read
// notifications, it reads nothing.
func (s *Section) list(ctx context.Context, q notifications.ListQuery, again bool) (core.Page[core.Notification], error) {
	// The section says why in place of the list, so the refusal isn't a
	// failure of the list.
	if s.voice.Token.Check(core.NeedNotifications) != nil {
		return core.Page[core.Notification]{}, nil
	}
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
	s.renderUnreadable()
}

// SetTheme builds the styles of the rows and restyles the list.
func (s *Section) SetTheme(t ui.Theme) {
	s.theme = t
	s.styles = newStyles(t)
	s.feed.SetStyles(t.Feed(s.icons))
	s.renderHeader()
	s.renderUnreadable()
}

// Focus makes the list react to keys.
func (s *Section) Focus() { s.feed.Focus() }

// Blur makes the list ignore keys, and stops the reads ahead of the
// threads, as the section leaves the screen.
func (s *Section) Blur() {
	s.feed.Blur()
	s.opener.Stop()
}

// KeyLayers implements ui.Keyed: the section's own keys, with the one
// that clears the filter only while there is one and the marks only while
// the token may mark, and then the list's.
func (s *Section) KeyLayers() []keyhelp.Layer {
	k := s.keys
	k.ClearFilter.SetEnabled(k.ClearFilter.Enabled() && s.filtered())
	g := s.gate()
	k.MarkRead = g.Gated(k.MarkRead, ui.ActMarkRead, nil)
	k.MarkDone = g.Gated(k.MarkDone, ui.ActMarkRead, nil)
	k.MarkAllRead = g.Gated(k.MarkAllRead, ui.ActMarkRead, nil)
	return []keyhelp.Layer{keyhelp.FromHelp(ui.NotificationsTitle, k, false), keyhelp.FromHelp("list", s.feed.KeyMap(), false)}
}
