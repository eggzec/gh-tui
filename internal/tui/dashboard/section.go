// Package dashboard is the screen the app opens on: the viewer's profile,
// the repository of the current directory and their pinned ones, their
// repositories and those of their organizations, the work waiting on them,
// their unread notifications and their contribution calendar, each in a
// pane of its own.
package dashboard

import (
	"context"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/dashboard"
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/threads"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Service is what the section needs of the dashboard service.
type Service interface {
	CachedHeader() (core.Header, bool)
	Header(ctx context.Context, q dashboard.HeaderQuery) (core.Header, error)
	CachedWork(q dashboard.WorkQuery) (core.Work, bool)
	Work(ctx context.Context, q dashboard.WorkQuery) (core.Work, error)
	CachedContributions() (core.Contributions, bool)
	Contributions(ctx context.Context, q dashboard.ContributionsQuery) (core.Contributions, error)
	CachedRepos(q dashboard.ReposQuery) (core.Page[core.Repo], bool)
	// The Fresh reads report whether reading costs no request, without
	// I/O.
	FreshHeader() bool
	FreshWork(q dashboard.WorkQuery) bool
	FreshContributions() bool
	FreshRepos(q dashboard.ReposQuery) bool
	Repos(ctx context.Context, q dashboard.ReposQuery) (core.Page[core.Repo], error)
	CachedAllRepos(q dashboard.ReposQuery, limit int) (core.Page[core.Repo], bool)
	// AllRepos reads the pages of q's owner up to limit repositories, the
	// cached ones without a request, for the filter.
	AllRepos(ctx context.Context, q dashboard.ReposQuery, limit int) (core.Page[core.Repo], error)
	// Invalidate marks everything cached stale, so that the reads after
	// it ask GitHub.
	Invalidate()
}

// Inbox is what the section needs of the notifications service: the first
// page of the default inbox, which holds the unread threads.
type Inbox interface {
	CachedList(q notifications.ListQuery) (core.Page[core.Notification], bool)
	List(ctx context.Context, q notifications.ListQuery) (core.Page[core.Notification], error)
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

// Marker marks a thread read, as the notifications service does, for the
// notifications pane to mark a thread it opens.
type Marker interface {
	MarkRead(id string) *optimistic.Op
}

// WithInbox shows the unread notifications of in. If in is a Marker too,
// opening a thread marks it read, as the notifications screen does. Without
// it the notifications pane says there are none to show.
func WithInbox(in Inbox) Option {
	return func(s *Section) {
		s.inbox = in
		s.marker, _ = in.(Marker)
	}
}

// WithOpener opens the threads of the notifications pane, and reads them
// ahead while the dashboard is on view, with o, which the notifications
// screen may share. By default the pane has one that reads nothing ahead.
func WithOpener(o *threads.Opener) Option {
	return func(s *Section) {
		if o != nil {
			s.opener = o
		}
	}
}

// WithHere shows repo, the repository of the current directory, as the
// first card of the pinned pane, where the current_repo key opens it. get
// reads the rest of the card, such as its description, in a command; it
// may be nil.
func WithHere(repo core.RepoRef, get func(ctx context.Context, repo core.RepoRef) (core.Repo, error)) Option {
	return func(s *Section) {
		s.here, s.getHere = repo, get
	}
}

// WithHost sets the web host of the user's GitHub, with its port if it has
// one, whose pages the dashboard opens. It defaults to github.com.
func WithHost(host string) Option {
	return func(s *Section) { s.host = host }
}

// WithGlyph sets the glyph of a day in the contribution calendar. It must
// be one cell wide; others keep the calendar's default.
func WithGlyph(glyph string) Option {
	return func(s *Section) { s.glyph = glyph }
}

// WithContributions shows the contributions of the last days days, or of
// the year GitHub reports for 0. The default is 90.
func WithContributions(days int) Option {
	return func(s *Section) { s.calDays = max(days, 0) }
}

// WithIcons sets the glyphs that mark repositories, languages and the
// states of issues and pull requests. The default is the Nerd Font set.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// paneID names a pane of the dashboard. They are numbered in this order,
// the order they are read in.
type paneID int

const (
	pinnedPane paneID = iota
	reposPane
	workPane
	calendarPane
	inboxPane
	numPanes
)

var (
	paneTitles = [numPanes]string{"Pinned", "Repositories", "Waiting on you", "Contributions", "Notifications"}
	// shortTitles name the panes that aren't on view in a narrow frame.
	shortTitles = [numPanes]string{"Pinned", "Repos", "Work", "Calendar", "Inbox"}
)

var lastID atomic.Int64

// Section is the dashboard. Create one with New.
type Section struct {
	id     int64
	ctx    context.Context
	svc    Service
	inbox  Inbox
	marker Marker
	opener *threads.Opener
	// ahead reads the work ahead, if prefetch is set.
	prefetch *prefetch
	ahead    *ui.Ahead[details.Key]
	keys     KeyMap
	now      func() time.Time
	offline  *ui.Offline
	voice    ui.Voice
	glyph    string
	icons    ui.Icons
	// host is the web host of the user's GitHub, for the links it opens.
	host string
	// links keeps the links of the rows, which are drawn again on every
	// change.
	links termtext.Links
	// calDays is the range of the calendar, 0 for the year.
	calDays int

	here    core.RepoRef
	getHere func(ctx context.Context, repo core.RepoRef) (core.Repo, error)

	// gen counts refreshes; replies to reads of an earlier one are
	// dropped.
	gen     int
	started bool
	focused bool
	focus   paneID

	header   read[core.Header]
	work     read[core.Work]
	contribs read[core.Contributions]
	notes    read[core.Page[core.Notification]]
	hereRepo read[core.Repo]

	pinned cards
	repos  repoTabs
	tasks  workList
	// threads are the unread notifications in their pane.
	threads inboxList
	cal     calendar.Model

	width, height int
	wide          bool
	// zoom shows the focused pane alone, as a narrow dashboard does.
	zoom bool
	// head is the profile above the panes, boxes are the sizes of the
	// panes, and frames the panes rendered in their frames.
	head   []string
	boxes  [numPanes]box
	frames [numPanes][]string
	theme  ui.Theme
	st     styles
	// view is rendered whenever the state changes, so View is free.
	view string
}

// read is what the section knows of one read: the value it shows, whether
// it has one, and the error of the last attempt.
type read[V any] struct {
	value V
	ok    bool
	err   error
	// loading is set while a read is in flight.
	loading bool
}

var (
	_ ui.Section    = (*Section)(nil)
	_ ui.Filterable = (*Section)(nil)
	_ ui.Revisiter  = (*Section)(nil)
)

// New returns the dashboard, which reads through svc and binds the actions
// in keys. ctx bounds every request it makes.
func New(ctx context.Context, svc Service, keys map[string][]string, opts ...Option) *Section {
	s := &Section{
		id:      lastID.Add(1),
		ctx:     ctx,
		svc:     svc,
		keys:    newKeyMap(keys),
		now:     time.Now,
		offline: new(ui.Offline),
		voice:   ui.NewVoice(keys, ""),
		glyph:   config.DefaultCalendarGlyph,
		icons:   ui.NewIcons(config.IconsNerd),
		calDays: 90,
		// Finding a repository is what the dashboard is most often for.
		focus: reposPane,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.opener == nil {
		s.opener = threads.New(ctx)
	}
	if p := s.prefetch; p != nil {
		s.ahead = details.NewAhead("work", p.pulls, p.issues, aheadRows, p.delay)
		s.ahead.Reset(ctx)
	}
	s.cal = calendar.New(
		calendar.WithGlyph(s.glyph),
		calendar.WithRange(s.calDays),
		calendar.WithEmptyText("Loading contributions…"),
	)
	s.repos = newRepoTabs(s)
	s.tasks.now = s.now
	s.pinned.here = s.here
	s.paintCached()
	s.SetTheme(ui.NewTheme(defaultPalette(), true))
	return s
}

// paintCached shows what the caches hold, without I/O, so that the
// dashboard paints at once.
func (s *Section) paintCached() {
	if h, ok := s.svc.CachedHeader(); ok {
		s.header.value, s.header.ok = h, true
		s.setHeader()
	}
	if w, ok := s.svc.CachedWork(dashboard.WorkQuery{}); ok {
		s.work.value, s.work.ok = w, true
		s.tasks.set(w)
	}
	if c, ok := s.svc.CachedContributions(); ok {
		s.contribs.value, s.contribs.ok = c, true
		s.setContributions()
	}
	s.readInboxCache()
}

// readInboxCache shows the inbox as the notifications service has it
// cached, which the notifications section and the polls keep current.
func (s *Section) readInboxCache() {
	if s.inbox == nil {
		return
	}
	if p, ok := s.inbox.CachedList(notifications.ListQuery{}); ok {
		s.notes.value, s.notes.ok, s.notes.err = p, true, nil
		s.setInbox()
	}
}

// Title returns the title of the section.
func (s *Section) Title() string { return ui.DashboardTitle }

// Init reads everything the dashboard shows.
func (s *Section) Init() tea.Cmd {
	s.started = true
	cmd := tea.Batch(s.load(), s.repos.start())
	s.render()
	return cmd
}

// SetSize sets the size of the dashboard and lays its panes out.
func (s *Section) SetSize(width, height int) {
	s.width, s.height = max(width, 0), max(height, 0)
	s.layout()
	s.render()
}

// SetTheme builds the styles of the dashboard and restyles its bubbles.
func (s *Section) SetTheme(t ui.Theme) {
	s.theme = t
	s.st = newStyles(t)
	s.cal.SetStyles(t.Calendar())
	s.repos.setTheme(t)
	s.render()
}

// Focus makes the focused pane react to keys, and shows the inbox as it is
// cached now, which may have changed while another screen was on view.
func (s *Section) Focus() {
	s.focused = true
	s.readInboxCache()
	s.focusPane(s.focus)
	s.render()
}

// Blur makes every pane ignore keys, and stops the reads ahead of the
// inbox's threads and of the work, as the dashboard leaves the screen.
func (s *Section) Blur() {
	s.focused = false
	s.opener.Stop()
	s.ahead.Reset(s.ctx)
	s.repos.blur()
	s.cal.Blur()
	s.render()
}

// View returns the dashboard, rendered when its state last changed.
func (s *Section) View() string { return s.view }

// Help lists the keys of the dashboard for the help line.
func (s *Section) Help() help.KeyMap { return ui.Hints{Layers: s.KeyLayers()} }

// Focused returns the number of the focused pane, from 0.
func (s *Section) Focused() int { return int(s.focus) }

func defaultPalette() config.Palette {
	p, _ := config.Default().Palette(true)
	return p
}
