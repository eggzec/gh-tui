// Package owner is the page of a user or an organization: who they are,
// the repositories they pinned and the lists of the page, each in a pane
// of its own. It keeps the pages opened one from another, so that the back
// key goes back through them.
package owner

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Service is what the section needs of the owners service.
type Service interface {
	CachedHeader(login string) (core.Owner, bool)
	// FreshHeader and FreshRepos report whether reading costs no
	// request, without I/O.
	FreshHeader(login string) bool
	Header(ctx context.Context, q owners.HeaderQuery) (core.Owner, error)
	FreshRepos(q owners.ReposQuery) bool
	Repos(ctx context.Context, q owners.ReposQuery) (core.Page[core.Repo], error)
	CachedAllRepos(q owners.ReposQuery, limit int) (core.Page[core.Repo], bool)
	// AllRepos reads the pages of q's owner up to limit repositories, the
	// cached ones without a request, for the filter.
	AllRepos(ctx context.Context, q owners.ReposQuery, limit int) (core.Page[core.Repo], error)
	// FreshStars, FreshPeople and FreshTeams report whether reading the
	// page costs no request, without I/O.
	FreshStars(q owners.StarsQuery) bool
	Stars(ctx context.Context, q owners.StarsQuery) (core.Page[core.Repo], error)
	FreshPeople(q owners.PeopleQuery) bool
	People(ctx context.Context, q owners.PeopleQuery) (core.Page[core.Person], error)
	FreshTeams(q owners.TeamsQuery) bool
	// Teams fails with an error matching core.ErrForbidden for someone
	// outside the organization.
	Teams(ctx context.Context, q owners.TeamsQuery) (core.Page[core.Team], error)
	// InvalidateLogin marks what is cached of the account login stale,
	// so that the reads of it after ask GitHub.
	InvalidateLogin(login string)
	sideService
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

// WithIcons sets the glyphs that mark repositories and languages. Without
// it, the icons are the config's default.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// WithDates sets how dates read, as ui.date_format says. The default is
// as ages.
func WithDates(d ui.Dates) Option {
	return func(s *Section) { s.dates = d }
}

// WithAvatars draws the avatar of the account beside the profile with a,
// in a box of ui.AvatarLarge, which makes the profile as tall as the box.
// Without it, or where the terminal shows no images, the profile keeps
// its two lines.
func WithAvatars(a *ui.Images) Option {
	return func(s *Section) { s.avatars = a }
}

// WithHost sets the web host of the user's GitHub, with its port if it has
// one, whose pages the section opens. It defaults to github.com.
func WithHost(host string) Option {
	return func(s *Section) { s.host = host }
}

// WithDefaultTab sets the tab of the list pane that a page opens on, as
// owner.default_tab names it, or, for the README, focuses its pane. A tab
// the page doesn't have opens it on the repositories, which is the
// default.
func WithDefaultTab(name string) Option {
	return func(s *Section) {
		s.defaultTab = tabFor(name)
		s.sc.focus = focusFor(name)
	}
}

// paneID names a pane of the page. They are numbered in this order.
type paneID int

const (
	pinnedPane paneID = iota
	listPane
	readmePane
	// calendarPane is a user's only.
	calendarPane
	numPanes
)

// paneTitles name the panes. The list pane goes by the title of its tab
// instead, and the README by its path once read.
var paneTitles = [numPanes]string{"Pinned", "", "README", "Contributions"}

// maxBack is the most pages the back key goes back through, so that a long
// walk from person to person keeps a bounded number of lists.
const maxBack = 20

var lastID atomic.Int64

// Section is the page of a user or an organization. Create one with New,
// and give it the account to show with a ui.OwnerMsg.
type Section struct {
	id    int64
	ctx   context.Context
	svc   Service
	keys  KeyMap
	now   func() time.Time
	voice ui.Voice
	icons ui.Icons
	// dates tell when the repositories were updated.
	dates ui.Dates
	// host is the web host of the user's GitHub, for the links it opens.
	host string
	// avatars draws the avatar of the account beside the profile.
	avatars *ui.Images
	// links keeps the links of the names, which are drawn again on every
	// change.
	links termtext.Links
	// defaultTab is the tab of the list pane each page opens on.
	defaultTab tab
	// sc is what the panes beside the list share across the pages.
	sc sideConf
	// ahead reads ahead what the page on view may open next, as layers,
	// the settings it starts with, say: the accounts through svc and the
	// repositories through landing. slots bound its reads with those of
	// other pages.
	ahead   aheads
	layers  config.PrefetchLayers
	landing Landing
	slots   *ui.Slots
	// viewer is the user's login, whose own row isn't read ahead.
	viewer string

	started bool
	focused bool
	// page is the page on view, or nil before the first, and back the
	// pages it was opened from, the latest last.
	page *page
	back []*page

	width, height int
	wide          bool
	// zoom shows the focused pane alone, as a narrow page does.
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

// page is the page of one account: what was read of it, and where its
// cursors are, which going back to it finds as they were.
type page struct {
	login string
	// gen counts the refreshes of the page; replies to reads of an
	// earlier one are dropped.
	gen    int
	header read[core.Owner]
	pinned ownerui.Cards
	// lists are the lists of the tabs, made once the header says whether
	// the account is a user or an organization, and read once their tab
	// is on view.
	lists [numTabs]lister
	// side is what the panes beside the list show.
	side  side
	focus paneID
	tab   tab
}

// list returns the list of the tab on view, or nil while there is none.
func (p *page) list() lister {
	if p == nil {
		return nil
	}
	return p.lists[p.tab]
}

// repos returns the list of the repositories, or nil while there is none.
func (p *page) repos() *repoList {
	l, _ := p.lists[reposTab].(*repoList)
	return l
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
	_ ui.Selector   = (*Section)(nil)
)

// New returns the page, which reads through svc and binds the actions in
// keys. ctx bounds every request it makes.
func New(ctx context.Context, svc Service, keys config.Keymap, opts ...Option) *Section {
	def := config.Default()
	s := &Section{
		id:         lastID.Add(1),
		ctx:        ctx,
		svc:        svc,
		keys:       newKeyMap(keys),
		now:        time.Now,
		voice:      ui.NewVoice(keys, ""),
		icons:      ui.NewIcons(def.UI.Icons),
		defaultTab: tabFor(def.Owner.DefaultTab),
		sc:         newSideConf(keys),
	}
	for _, opt := range opts {
		opt(s)
	}
	// Bubbles copy the voice, and read the icons through it.
	s.voice.Icons = &s.icons
	s.newAheads()
	s.setPrefetch(s.layers)
	s.SetTheme(ui.NewTheme(defaultPalette(), true))
	return s
}

// Title returns the title of the section.
func (s *Section) Title() string { return ui.OwnerTitle }

// Login returns the login of the account on view, as GitHub spells it, or
// "" before one is.
func (s *Section) Login() string {
	if s.page == nil {
		return ""
	}
	if s.page.header.ok && s.page.header.value.Profile.Login != "" {
		return s.page.header.value.Profile.Login
	}
	return s.page.login
}

// Init reads the page on view.
func (s *Section) Init() tea.Cmd {
	s.started = true
	cmd := s.load(s.page)
	s.render()
	return cmd
}

// SetSize sets the size of the page and lays its panes out.
func (s *Section) SetSize(width, height int) {
	s.width, s.height = max(width, 0), max(height, 0)
	s.layout()
	s.render()
}

// SetTheme builds the styles of the page and restyles its lists.
func (s *Section) SetTheme(t ui.Theme) {
	s.voice.Icons = &s.icons
	s.theme = t
	s.st = newStyles(t, s.icons)
	s.errs = t.Errors(s.icons)
	for _, p := range s.pages() {
		for _, l := range p.lists {
			if l != nil {
				l.feed().SetStyles(t.Feed(s.icons))
			}
		}
	}
	s.themeSide()
	s.render()
}

// Focus makes the focused pane react to keys.
func (s *Section) Focus() {
	s.focused = true
	s.focusPane()
	s.render()
}

// Blur makes every pane ignore keys, and stops the reads ahead, as the
// page leaves the screen.
func (s *Section) Blur() {
	s.focused = false
	s.stopAhead()
	s.focusPane()
	s.render()
}

// View returns the page, rendered when its state last changed.
func (s *Section) View() string { return s.view }

// open shows the page of login, which goto found GitHub has. Opened from
// the page of another account, it keeps that one to go back to; opened
// from another screen, it starts a new way back.
func (s *Section) open(login string) tea.Cmd {
	if !s.focused {
		s.back = nil
	}
	if s.page != nil && strings.EqualFold(s.page.login, login) {
		return nil
	}
	if s.focused && s.page != nil {
		s.back = append(s.back, s.page)
		if len(s.back) > maxBack {
			s.back = s.back[len(s.back)-maxBack:]
		}
	}
	s.page = s.newPage(login)
	s.layout()
	s.focusPane()
	return s.load(s.page)
}

// goBack shows the page the one on view was opened from, and reports
// whether there was one.
func (s *Section) goBack() bool {
	if len(s.back) == 0 {
		return false
	}
	s.blurPage(s.page)
	s.page = s.back[len(s.back)-1]
	s.back = s.back[:len(s.back)-1]
	s.layout()
	s.focusPane()
	return true
}

// newPage returns the page of login, as the cache has it.
func (s *Section) newPage(login string) *page {
	p := &page{login: login, focus: s.sc.focus, tab: s.defaultTab}
	if o, ok := s.svc.CachedHeader(login); ok {
		p.header.value, p.header.ok = o, true
		s.setHeader(p)
	}
	return p
}

// pages returns the page on view and those to go back to.
func (s *Section) pages() []*page {
	if s.page == nil {
		return s.back
	}
	return append(slices.Clip(s.back), s.page)
}

func defaultPalette() config.Palette {
	p, _ := config.Default().Palette(true)
	return p
}
