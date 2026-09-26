// Package tui is the root of the program. It lays the sections out on
// four screens, the dashboard, the repository screen with its panes, the
// notifications screen and the search page, draws the header, the help
// line and toasts, opens modals such as the history over them, runs the
// commands of the command line, and routes messages between them all.
// The sections themselves live in their own packages and share the ui
// package.
package tui

import (
	"context"
	"slices"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
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
	// Dashboard fills the dashboard, which the app opens on unless it is
	// given a repository. It draws its own panes, and handles the keys
	// that move between them.
	Dashboard ui.Section
	// Search fills the search page, which the search key shows from any
	// screen. It draws its own frames too.
	Search ui.Section
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
	dash  *pane
	srch  *pane
	// all holds the panes of every screen.
	all []*pane
	// screen is the screen on view, and focus the focused pane of the
	// repository screen. back is the screen before it, which the keys that
	// show the notifications and the dashboard go back to.
	screen screen
	back   screen
	focus  int
	// zoom shows the focused pane of the repository screen alone, as a
	// narrow terminal does. The dashboard keeps its own.
	zoom bool
	// pending holds the commands the sections returned for the repository
	// of WithRepo, for Init to run.
	pending tea.Cmd
	// modal is open over the screens, or nil. Opening another replaces it.
	modal ui.Modal

	repo   core.RepoRef
	branch string
	// base is what the files of repo are shown at, set by a ui.BaseMsg;
	// its Ref is empty for the head of the default branch.
	base  ui.BaseMsg
	badge string

	toast toast.Model
	help  help.Model
	// line is the command line, which takes the place of the help line
	// while it is open.
	line cmdline.Model
	// going is the goto waiting for GitHub, or nil, and gotoSeq numbers
	// them, so that the answer to one replaced is ignored. spin shows in
	// the footer while one waits.
	going   *going
	gotoSeq int
	spin    spinner.Model
	theme   ui.Theme
	st      styles
	// header is rendered whenever what it shows changes.
	header string

	width, height int

	sync      func(ctx context.Context) (ui.SyncMsg, bool)
	setActive func(active bool)
	open      func(url string) error
	watchRepo func(repo core.RepoRef)
	repoInfo  func(ctx context.Context, repo core.RepoRef) (core.Repo, error)
	// repos checks that a repository exists before goto opens it, and
	// kinds tells an issue from a pull request. host is the one whose
	// links goto opens, and unreachable tells whether an error means
	// GitHub couldn't be reached.
	repos       Repos
	kinds       Kinds
	host        string
	unreachable func(ctx context.Context, err error) bool
	// recall is what the command line completes from, with recent, the
	// repositories selected in this session, the latest first.
	recall Recall
	recent []core.RepoRef
	// hist keeps the lines of the command line between sessions, or is
	// nil.
	hist *historyKeeper
	// history opens the history modal of a repository, and actions its
	// Actions modal.
	history History
	actions Actions
	// commit opens the history on a commit, and release the modal of a
	// release.
	commit  Commit
	release Release
	// warnings are shown as toasts once the app starts.
	warnings []string
}

// Option configures a Model.
type Option func(*Model)

// WithRepo sets the repository the app opens with, on the repository
// screen. Without it the app opens on the dashboard, or on the
// notifications screen when it has none, until the user picks a
// repository.
func WithRepo(repo core.RepoRef) Option {
	return func(m *Model) { m.repo = repo }
}

// WithRepoInfo sets the function that reads a repository, so that the
// header can show its default branch and the sections learn what the
// viewer may do in it, from a ui.CapsMsg. It is called in a command
// whenever a repository is selected.
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

// History returns the modal that shows the history of repo, and the
// command that loads it once it is open. defaultBranch is the repository's
// default branch, or empty until the app has read it, and base is what its
// files are shown at, with an empty Ref for the head of the default branch.
// The modal sets another base with a ui.BaseMsg.
type History func(ctx context.Context, repo core.RepoRef, defaultBranch string, base ui.BaseMsg) (ui.Modal, tea.Cmd)

// WithHistory sets the function that opens the history of the selected
// repository, with the history key on the repository screen. Without it
// the key does nothing.
func WithHistory(open History) Option {
	return func(m *Model) { m.history = open }
}

// Actions returns the modal that shows the workflow runs of repo that f
// selects, and the command that loads it once it is open.
type Actions func(ctx context.Context, repo core.RepoRef, f core.RunFilter) (ui.Modal, tea.Cmd)

// WithActions sets the function that opens the workflow runs of the
// selected repository, with the actions key on the repository screen.
// Without it the key does nothing.
func WithActions(open Actions) Option {
	return func(m *Model) { m.actions = open }
}

// Commit returns the history of repo opened on commit sha, and the command
// that loads it once it is open. defaultBranch is the repository's default
// branch, or empty when the app hasn't read it.
type Commit func(ctx context.Context, repo core.RepoRef, sha, defaultBranch string) (ui.Modal, tea.Cmd)

// WithCommit sets the function that opens the history on a commit, which a
// ui.OpenCommitMsg asks for. Without it the message does nothing.
func WithCommit(open Commit) Option {
	return func(m *Model) { m.commit = open }
}

// Release returns the modal that shows release id of repo, and the command
// that loads it once it is open. url is the page to open on GitHub until
// the release is read.
type Release func(ctx context.Context, repo core.RepoRef, id int64, url string) (ui.Modal, tea.Cmd)

// WithRelease sets the function that opens a release, which a
// ui.OpenReleaseMsg asks for. Without it the message does nothing.
func WithRelease(open Release) Option {
	return func(m *Model) { m.release = open }
}

// WithBrowser sets the function that opens a URL in the browser.
func WithBrowser(open func(url string) error) Option {
	return func(m *Model) { m.open = open }
}

// WithWarning shows text in a warning toast once the app starts, for a
// problem at startup that the app works around, such as a cache it
// couldn't open.
func WithWarning(text string) Option {
	return func(m *Model) { m.warnings = append(m.warnings, text) }
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
		line:  newLine(cfg.Keys),
		spin:  newSpinner(),
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
	m.all = slices.Clip(m.panes)
	if layout.Notifications != nil {
		m.notif = &pane{section: layout.Notifications, label: layout.Notifications.Title()}
		m.all = append(m.all, m.notif)
	}
	if layout.Dashboard != nil {
		m.dash = &pane{section: layout.Dashboard, bare: true}
		m.all = append(m.all, m.dash)
	}
	if layout.Search != nil {
		m.srch = &pane{section: layout.Search, bare: true}
		m.all = append(m.all, m.srch)
	}
	for _, opt := range opts {
		opt(m)
	}
	m.line.SetComplete(m.complete)

	switch {
	case m.repo != (core.RepoRef{}):
		m.screen = repoScreen
	case m.dash != nil:
		m.screen = dashScreen
	case m.notif != nil:
		m.screen = notifScreen
	default:
		m.screen = repoScreen
	}
	m.back = m.screen
	if m.repo != (core.RepoRef{}) {
		m.remember(m.repo)
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
	cmds := []tea.Cmd{tea.RequestBackgroundColor, m.pending, m.startScreen(), m.listen(), m.loadRepoInfo(), m.loadHistory()}
	m.pending = nil
	for _, w := range m.warnings {
		cmds = append(cmds, ui.Notify(toast.Warning, w))
	}
	m.warnings = nil
	if m.notif != nil {
		cmds = append(cmds, m.notif.start())
	}
	return tea.Batch(cmds...)
}

// startScreen starts the sections of the screen on view that haven't
// started yet.
func (m *Model) startScreen() tea.Cmd {
	switch m.screen {
	case notifScreen:
		return m.notif.start()
	case dashScreen:
		return m.dash.start()
	case searchScreen:
		return m.srch.start()
	case repoScreen:
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
		ctx, end := obs.Begin(ctx, "repo.info")
		r, err := get(ctx, ref)
		end(err, "span", "tui", "repo", ref.String())
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
	m.line.SetStyles(m.theme.Cmdline())
	m.spin.Style = m.theme.Accent
	for _, p := range m.all {
		p.section.SetTheme(m.theme)
	}
	if m.modal != nil {
		m.modal.SetTheme(m.theme)
	}
	m.drawFrames()
	m.drawHeader()
}

// updateBadges takes the badge of the notifications, for the header, and
// the chips of the panes, for their titles.
func (m *Model) updateBadges() {
	m.updateChips()
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

// updateChips puts the chips of each pane of the repository screen after
// its title, such as the filters of its list, and redraws the frames whose
// titles changed.
func (m *Model) updateChips() {
	for i, p := range m.panes {
		c, ok := p.section.(ui.Chipper)
		if !ok {
			continue
		}
		label := paneLabel(i, p.section.Title())
		if chips := c.Chips(); chips != "" {
			label += " · " + chips
		}
		if label != p.label {
			p.label = label
			m.drawFrame(p)
		}
	}
}
