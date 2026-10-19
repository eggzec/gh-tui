// Package notifications is the section of the user's inbox. It lists the
// threads, opens what each is about in the app through a threads.Opener,
// which reads them ahead too, and marks them read or done.
package notifications

import (
	"context"
	"maps"
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

// States reads the state of the subjects of threads: pull requests, issues
// and discussions. The section works without it, and its rows are the same.
type States interface {
	// CachedStates returns what is held in memory, without I/O.
	CachedStates(items []core.Notification) map[core.SubjectRef]core.SubjectState
	// States returns the states held, and asks for those that are due.
	States(ctx context.Context, items []core.Notification) (map[core.SubjectRef]core.SubjectState, error)
}

// statesMsg carries the states a read answered, for the section to keep.
type statesMsg struct {
	states map[core.SubjectRef]core.SubjectState
}

// WithStates has the section read the state of the threads it lists from st,
// once their rows show. By default it reads none.
func WithStates(st States) Option {
	return func(s *Section) { s.reader = st }
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
	// reader reads the states of the subjects, if set, and states holds
	// them. Both are used in Update only.
	reader States
	states map[core.SubjectRef]core.SubjectState

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

// loaded runs when the feed stores a page: it keeps the states held in
// memory at once, and returns the command that reads the rest, so that the
// rows show before their states do.
func (s *Section) loaded(items []core.Notification) tea.Cmd {
	if s.reader == nil || len(items) == 0 {
		return nil
	}
	maps.Copy(s.states, s.reader.CachedStates(items))
	return func() tea.Msg {
		// A failure is logged by the service; the rows stay as they are.
		got, _ := s.reader.States(s.ctx, items)
		return statesMsg{states: got}
	}
}

// State returns the state of the subject of n, or false if it isn't known.
func (s *Section) State(n core.Notification) (core.SubjectState, bool) {
	st, ok := s.states[core.SubjectRef{Repo: n.Repo, Number: n.Subject.Number, Type: n.Subject.Type}]
	return st, ok
}

// New returns the section, which reads through svc and binds the actions
// in keys. ctx bounds every request it makes.
func New(ctx context.Context, svc Service, keys config.Keymap, opts ...Option) *Section {
	s := &Section{ctx: ctx, svc: svc, keys: newKeyMap(keys), now: time.Now, states: map[core.SubjectRef]core.SubjectState{}, voice: ui.NewVoice(keys, ""), icons: ui.NewIcons(config.Default().UI.Icons)}
	for _, opt := range opts {
		opt(s)
	}
	// Bubbles copy the voice, and read the icons through it.
	s.voice.Icons = &s.icons
	if s.opener == nil {
		s.opener = threads.New(ctx)
	}
	if !s.opener.MarksRead() {
		s.keys.Select.SetHelp(s.keys.Select.Help().Key, "open")
	}
	s.feed = feed.New(ui.FeedPages("list.notifications", s.query, s.list), s.render,
		feed.WithContext(ctx),
		feed.WithOnLoad(s.loaded),
		feed.WithKey(func(n core.Notification) string { return n.ID }),
		feed.WithKeyMap(s.keys.feed),
		feed.WithMarkKeys(s.keys.mark),
		feed.WithPromptKeys(s.keys.search),
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
	s.voice.Icons = &s.icons
	s.opener.SetIcons(s.icons)
	s.theme = t
	s.styles = newStyles(t, s.icons)
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
	list, prompting := ui.FeedLayer(s.feed)
	if prompting {
		return []keyhelp.Layer{list}
	}
	return []keyhelp.Layer{ui.MergeLayers(ctxScreen, keyhelp.FromHelp("", k, false), list)}
}

// Capturing implements ui.Capturer: the list takes every key while its
// find or filter prompt is open.
func (s *Section) Capturing() bool { return s.feed.Capturing() }
