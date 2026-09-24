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
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
)

// Service is what the section needs of the dashboard service.
type Service interface {
	CachedHeader() (core.Header, bool)
	Header(ctx context.Context) (core.Header, error)
	CachedWork(q dashboard.WorkQuery) (core.Work, bool)
	Work(ctx context.Context, q dashboard.WorkQuery) (core.Work, error)
	CachedContributions() (core.Contributions, bool)
	Contributions(ctx context.Context) (core.Contributions, error)
	CachedRepos(q dashboard.ReposQuery) (core.Page[core.Repo], bool)
	Repos(ctx context.Context, q dashboard.ReposQuery) (core.Page[core.Repo], error)
	CachedAllRepos(q dashboard.ReposQuery, limit int) (core.Page[core.Repo], bool)
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

// WithInbox shows the unread notifications of in. Without it the
// notifications pane says there are none to show.
func WithInbox(in Inbox) Option {
	return func(s *Section) { s.inbox = in }
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

// WithGlyph sets the glyph of a day in the contribution calendar. It must
// be one cell wide; others keep the calendar's default.
func WithGlyph(glyph string) Option {
	return func(s *Section) { s.glyph = glyph }
}

// paneID names a pane of the dashboard. They are numbered in this order.
type paneID int

const (
	pinnedPane paneID = iota
	reposPane
	workPane
	inboxPane
	calendarPane
	numPanes
)

var (
	paneTitles = [numPanes]string{"Pinned", "Repositories", "Waiting on you", "Notifications", "Contributions"}
	// shortTitles name the panes that aren't on view in a narrow frame.
	shortTitles = [numPanes]string{"Pinned", "Repos", "Work", "Inbox", "Calendar"}
)

var lastID atomic.Int64

// Section is the dashboard. Create one with New.
type Section struct {
	id      int64
	ctx     context.Context
	svc     Service
	inbox   Inbox
	keys    KeyMap
	now     func() time.Time
	offline *ui.Offline
	glyph   string

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
	cal    calendar.Model

	width, height int
	wide          bool
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
	_ ui.Section  = (*Section)(nil)
	_ ui.Capturer = (*Section)(nil)
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
		glyph:   config.DefaultCalendarGlyph,
		// Finding a repository is what the dashboard is most often for.
		focus: reposPane,
	}
	for _, opt := range opts {
		opt(s)
	}
	s.cal = calendar.New(
		calendar.WithGlyph(s.glyph),
		calendar.WithEmptyText("Loading contributions…"),
	)
	s.repos = newRepoTabs(s)
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

// Capturing reports whether the section takes every key, which it does
// while the repositories are filtered.
func (s *Section) Capturing() bool { return s.repos.filtering }

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

// Blur makes every pane ignore keys.
func (s *Section) Blur() {
	s.focused = false
	s.repos.blur()
	s.cal.Blur()
	s.render()
}

// View returns the dashboard, rendered when its state last changed.
func (s *Section) View() string { return s.view }

// Help returns the keys of the focused pane, then those of the dashboard.
func (s *Section) Help() help.KeyMap {
	return helpKeys{k: s.keys, pane: s.focus, repos: &s.repos, cal: s.cal.KeyMap(), here: s.here != (core.RepoRef{})}
}

// Focused returns the number of the focused pane, from 0.
func (s *Section) Focused() int { return int(s.focus) }

func defaultPalette() config.Palette {
	p, _ := config.Default().Palette(true)
	return p
}
