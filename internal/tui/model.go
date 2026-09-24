// Package tui is the root of the program. It lays the sections out on two
// screens, the repository screen with its panes and the notifications
// screen, draws the header, the help line and toasts, opens modals such as
// the search over them, and routes messages between them all. The sections
// themselves live in their own packages and share the ui package.
package tui

import (
	"context"
	"slices"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Layout places the sections on the screens. A nil section leaves its place
// out.
type Layout struct {
	// Files fills the left of the repository screen, and Pulls and Issues
	// share its right, one above the other. They are panes 1, 2 and 3.
	Files, Pulls, Issues ui.Section
	// Notifications fills the notifications screen.
	Notifications ui.Section
}

// Model is the root model of the program.
type Model struct {
	ctx  context.Context
	cfg  config.Config
	keys KeyMap

	// panes are those of the repository screen, in the order focus cycles
	// through them. The first left of them are on the left.
	panes []*pane
	left  int
	notif *pane
	// all holds the panes of every screen.
	all []*pane
	// screen is the screen on view, and focus the focused pane of the
	// repository screen.
	screen screen
	focus  int
	// pending holds the commands the sections returned for the repository
	// of WithRepo, for Init to run.
	pending tea.Cmd
	// modals are open over the screens, the last one on top.
	modals []ui.Modal

	repo   core.RepoRef
	branch string
	badge  string

	toast toast.Model
	help  help.Model
	theme ui.Theme
	st    styles
	// header is rendered whenever what it shows changes.
	header string

	width, height int

	sync      func(ctx context.Context) (ui.SyncMsg, bool)
	setActive func(active bool)
	open      func(url string) error
	watchRepo func(repo core.RepoRef)
	repoInfo  func(ctx context.Context, repo core.RepoRef) (core.Repo, error)
	// search finds what the search modal lists; searchBox is that modal,
	// made the first time it opens.
	search    picker.Search
	searchBox *searchModal
}

// Option configures a Model.
type Option func(*Model)

// WithRepo sets the repository the app opens with, on the repository
// screen. Without it the app opens on the notifications screen, until the
// user picks a repository in the search.
func WithRepo(repo core.RepoRef) Option {
	return func(m *Model) { m.repo = repo }
}

// WithRepoInfo sets the function that reads a repository, so that the
// header can show its default branch. It is called in a command whenever
// a repository is selected.
func WithRepoInfo(get func(ctx context.Context, repo core.RepoRef) (core.Repo, error)) Option {
	return func(m *Model) { m.repoInfo = get }
}

// WithSync sets the source of sync events. next blocks until the data behind
// a key may have changed, and reports false once there are no more events.
func WithSync(next func(ctx context.Context) (ui.SyncMsg, bool)) Option {
	return func(m *Model) { m.sync = next }
}

// WithActivity sets the function told whether the terminal has focus, so
// background polling can slow down while the user looks elsewhere.
func WithActivity(setActive func(active bool)) Option {
	return func(m *Model) { m.setActive = setActive }
}

// WithRepoWatcher sets the function told which repository is selected, so
// that the app can poll it for changes. It is called with the repository of
// WithRepo and with every ui.RepoMsg, before the sections see it, and must
// not block.
func WithRepoWatcher(watch func(repo core.RepoRef)) Option {
	return func(m *Model) { m.watchRepo = watch }
}

// WithSearch sets the function the search modal lists results with. It is
// called for an empty query too, to offer something to start from. Items
// are grouped by their Kind, one of KindPinned, KindRepos, KindIssues and
// KindPulls, and a scope is one of the last three. Choosing an item whose
// Value is a core.RepoRef or core.Repo selects that repository; a
// core.SearchHit selects its repository or opens its issue or pull
// request. Without WithSearch the search key does nothing.
func WithSearch(search picker.Search) Option {
	return func(m *Model) { m.search = search }
}

// WithBrowser sets the function that opens a URL in the browser.
func WithBrowser(open func(url string) error) Option {
	return func(m *Model) { m.open = open }
}

// New returns the root model with the sections of layout. ctx bounds every
// request the app makes.
func New(ctx context.Context, cfg config.Config, layout Layout, opts ...Option) *Model {
	m := &Model{
		ctx:   ctx,
		cfg:   cfg,
		keys:  newKeyMap(cfg.Keys),
		toast: toast.New(),
		help:  help.New(),
	}
	if layout.Files != nil {
		m.panes, m.left = append(m.panes, &pane{section: layout.Files}), 1
	}
	for _, s := range []ui.Section{layout.Pulls, layout.Issues} {
		if s != nil {
			m.panes = append(m.panes, &pane{section: s})
		}
	}
	for i, p := range m.panes {
		p.label = paneLabel(i, p.section.Title())
	}
	if layout.Notifications != nil {
		m.notif = &pane{section: layout.Notifications, label: layout.Notifications.Title()}
		m.all = append(slices.Clip(m.panes), m.notif)
	} else {
		m.all = m.panes
	}
	for _, opt := range opts {
		opt(m)
	}

	if m.repo != (core.RepoRef{}) || m.notif == nil {
		m.screen = repoScreen
	} else {
		m.screen = notifScreen
	}
	if m.repo != (core.RepoRef{}) {
		if m.watchRepo != nil {
			m.watchRepo(m.repo)
		}
		// Sections take the repository before they start. What they ask
		// for in return runs from Init.
		cmds := make([]tea.Cmd, 0, len(m.all))
		for _, p := range m.all {
			cmds = append(cmds, p.section.Update(ui.RepoMsg{Repo: m.repo}))
		}
		m.pending = tea.Batch(cmds...)
	}
	if p := m.focused(); p != nil {
		p.setFocus(true)
	}
	// Assume a dark terminal until it tells us otherwise.
	m.applyTheme(true)
	return m
}

// Init asks for the terminal background, starts the sections on screen and
// the notifications, whose badge is on every screen, and listens for sync
// events.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.RequestBackgroundColor, m.pending, m.startScreen(), m.listen(), m.loadRepoInfo()}
	m.pending = nil
	if m.notif != nil {
		cmds = append(cmds, m.notif.start())
	}
	return tea.Batch(cmds...)
}

// startScreen starts the sections of the screen on view that haven't
// started yet.
func (m *Model) startScreen() tea.Cmd {
	if m.screen == notifScreen {
		return m.notif.start()
	}
	cmds := make([]tea.Cmd, 0, len(m.panes))
	for _, p := range m.panes {
		cmds = append(cmds, p.start())
	}
	return tea.Batch(cmds...)
}

func (m *Model) listen() tea.Cmd {
	if m.sync == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := m.sync(m.ctx)
		if !ok {
			return nil
		}
		return msg
	}
}

// repoInfoMsg carries what WithRepoInfo read about the repository.
type repoInfoMsg struct {
	repo core.Repo
	err  error
}

func (m *Model) loadRepoInfo() tea.Cmd {
	if m.repoInfo == nil || m.repo == (core.RepoRef{}) {
		return nil
	}
	get, ctx, ref := m.repoInfo, m.ctx, m.repo
	return func() tea.Msg {
		r, err := get(ctx, ref)
		if err == nil {
			// Callers may leave out what they were asked for.
			r.Ref = ref
		}
		return repoInfoMsg{repo: r, err: err}
	}
}

func (m *Model) applyTheme(dark bool) {
	p, err := m.cfg.Palette(dark)
	if err != nil {
		// A loaded config is validated, so this only guards a zero Config.
		p, _ = config.Default().Palette(dark)
	}
	m.theme = ui.NewTheme(p, dark)
	m.st = newStyles(m.theme)
	m.toast.SetStyles(m.theme.Toast())
	m.help.Styles = m.theme.Help()
	for _, p := range m.all {
		p.section.SetTheme(m.theme)
	}
	for _, mod := range m.modals {
		mod.SetTheme(m.theme)
	}
	if m.searchBox != nil && !m.isOpen(m.searchBox) {
		m.searchBox.SetTheme(m.theme)
	}
	m.drawFrames()
	m.drawHeader()
}

// updateBadges takes the badge of the notifications, for the header.
func (m *Model) updateBadges() {
	if m.notif == nil {
		return
	}
	b, ok := m.notif.section.(ui.Badger)
	if !ok {
		return
	}
	if badge := b.Badge(); badge != m.badge {
		m.badge = badge
		m.drawHeader()
	}
}
