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
	// FreshList reports whether reading q costs no request, without I/O.
	FreshList(q notifications.ListQuery) bool
	List(ctx context.Context, q notifications.ListQuery) (core.Page[core.Notification], error)
}

// Repos reads the repository of the current directory, for its card.
type Repos interface {
	Get(ctx context.Context, repo core.RepoRef) (core.Repo, error)
	// FreshGet reports whether reading repo costs no request, without
	// I/O.
	FreshGet(repo core.RepoRef) bool
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
// first card of the pinned pane, where the current_repo key opens it. r
// reads the rest of the card, such as its description, in a command; it
// may be nil.
func WithHere(repo core.RepoRef, r Repos) Option {
	return func(s *Section) {
		s.here, s.hereRepos = repo, r
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
// the year GitHub reports for 0. Without it, the calendar covers the
// config's default range.
func WithContributions(days int) Option {
	return func(s *Section) { s.calDays = max(days, 0) }
}

// WithIcons sets the glyphs that mark repositories, languages and the
// states of issues and pull requests. Without it, the icons are the
// config's default.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// WithDates sets how dates read, as ui.date_format says. The default is
// as ages.
func WithDates(d ui.Dates) Option {
	return func(s *Section) { s.dates = d }
}

// WithAvatars draws the viewer's avatar beside the profile with a, in a
// box of ui.AvatarLarge, which makes the profile as tall as the box.
// Without it, or where the terminal shows no images, the profile keeps
// its two lines.
func WithAvatars(a *ui.Images) Option {
	return func(s *Section) { s.avatars = a }
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
	// ahead reads the work ahead through readers, as workAhead says.
	// layers are the settings WithPrefetch gave, which New resolves, and
	// slots bound the reads of every pane with those of other pages.
	readers   *readers
	ahead     *ui.Ahead[details.Key]
	layers    config.PrefetchLayers
	workAhead config.Resolved
	slots     *ui.Slots
	// aheadRepos and aheadPinned read ahead, through landing, what
	// opening the repositories around the cursors of the repositories and
	// pinned panes reads first.
	landing     Landing
	aheadRepos  *ui.Ahead[core.RepoRef]
	aheadPinned *ui.Ahead[core.RepoRef]
	keys        KeyMap
	now         func() time.Time
	voice       ui.Voice
	glyph       string
	icons       ui.Icons
	// dates tell when what the panes list was updated.
	dates ui.Dates
	// host is the web host of the user's GitHub, for the links it opens.
	host string
	// avatars draws the viewer's avatar beside the profile.
	avatars *ui.Images
	// links keeps the links of the rows, which are drawn again on every
	// change.
	links termtext.Links
	// calDays is the range of the calendar, 0 for the year.
	calDays int

	here      core.RepoRef
	hereRepos Repos

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
	errs   ui.ErrorStyles
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
	def := config.Default()
	s := &Section{
		id:      lastID.Add(1),
		ctx:     ctx,
		svc:     svc,
		keys:    newKeyMap(keys),
		now:     time.Now,
		voice:   ui.NewVoice(keys, ""),
		glyph:   def.Dashboard.CalendarGlyph,
		icons:   ui.NewIcons(def.UI.Icons),
		calDays: def.Dashboard.ContributionDays(),
		// Finding a repository is what the dashboard is most often for.
		focus: reposPane,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.opener == nil {
		s.opener = threads.New(ctx)
	}
	s.newLandingAheads()
	s.setPrefetch(s.layers)
	s.cal = calendar.New(
		calendar.WithGlyph(s.glyph),
		calendar.WithRange(s.calDays),
	)
	s.repos = newRepoTabs(s)
	s.tasks.now, s.tasks.dates, s.tasks.ellipsis = s.now, s.dates, s.icons.Ellipsis
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

// cellGlyph returns the glyph of a day in the calendar: glyph, as
// configured, unless it is the default, which the icon set draws as its
// own cell.
func cellGlyph(glyph string, ic ui.Icons) string {
	if glyph == config.Default().Dashboard.CalendarGlyph {
		return ic.Cell
	}
	return glyph
}

// SetTheme builds the styles of the dashboard and restyles its bubbles.
func (s *Section) SetTheme(t ui.Theme) {
	s.theme = t
	s.st = newStyles(t, s.icons)
	s.errs = t.Errors(s.icons)
	s.cal.SetStyles(t.Calendar(s.icons))
	s.cal.SetGlyph(cellGlyph(s.glyph, s.icons))
	if !s.contribs.ok {
		s.cal.SetEmptyText("Loading contributions" + s.icons.Ellipsis)
	}
	if s.tasks.ellipsis != s.icons.Ellipsis {
		s.tasks.ellipsis = s.icons.Ellipsis
		s.tasks.wrap()
	}
	s.repos.setTheme(t, s.icons)
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
// inbox's threads, of the work and of the repositories, as the dashboard
// leaves the screen.
func (s *Section) Blur() {
	s.focused = false
	s.opener.Stop()
	// The notifications screen reads ahead as its own settings say.
	s.opener.FollowInbox(false)
	s.ahead.Reset(s.ctx)
	s.aheadRepos.Reset(s.ctx)
	s.aheadPinned.Reset(s.ctx)
	s.repos.blur()
	s.cal.Blur()
	s.render()
}

// View returns the dashboard, rendered when its state last changed.
func (s *Section) View() string { return s.view }

// Focused returns the number of the focused pane, from 0.
func (s *Section) Focused() int { return int(s.focus) }

func defaultPalette() config.Palette {
	p, _ := config.Default().Palette(true)
	return p
}
