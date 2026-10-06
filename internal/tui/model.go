// Package tui is the root of the program. It lays the sections out on
// five screens, the dashboard, the repository screen with its panes, the
// notifications screen, the search page and the page of a user or an
// organization, draws the header, the status
// bar and toasts, opens modals such as the history over them, runs the
// commands of the command line, and routes messages between them all.
// The sections themselves live in their own packages and share the ui
// package.
package tui

import (
	"context"
	"os/exec"
	"slices"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/statusbar"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/termtext"
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
	// Owner fills the page of a user or an organization, which goto
	// shows, given a ui.OwnerMsg for the account first. It draws its own
	// frames, handles the keys that move between its panes, and sends a
	// ui.BackMsg when it has no page left to go back to. If it is an
	// OwnerPage, the header names the account on view.
	Owner ui.Section
}

// Model is the root model of the program.
type Model struct {
	ctx context.Context
	cfg config.Config
	// file is the config gh-tui started with, as the file says and the
	// flags and environment raised the log level, which the set command
	// changes cfg from, for the session, and resets settings to.
	file config.Config
	// source is what the config file said, for the config command.
	source config.Source
	keys   KeyMap
	// term is what the terminal said of itself, for the log.
	term terminal
	// images finds out whether the terminal shows images, cells the size
	// of a cell in pixels when it does, and graphics is what they found.
	images   imageProbe
	cells    cellQuery
	graphics ui.Graphics
	// pics draws the images the sections show, such as avatars, or nothing
	// when nil, and picsDue is set while the views wait to draw those
	// arrived.
	pics    *ui.Images
	picsDue bool
	// quitStage is how far quitting got while the terminal held images.
	quitStage int
	// after sends a message after a while: tick, which tests replace.
	after func(d time.Duration, msg tea.Msg) tea.Cmd

	// panes are those of the repository screen, in the order focus cycles
	// through them. The first left of them are on the left.
	panes []*pane
	left  int
	notif *pane
	dash  *pane
	srch  *pane
	own   *pane
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
	// ownerAvatar is the address of the avatar of repo's owner, once its
	// read says.
	ownerAvatar string
	// base is what the files of repo are shown at, set by a ui.BaseMsg;
	// its Ref is empty for the head of the default branch.
	base  ui.BaseMsg
	badge string

	toast toast.Model
	// toastTimes is what the toasts were last given, to give them only a change.
	toastTimes config.Toast
	// status is the status bar: hints, of the keys of layers, on the
	// left, and stats, the rate limits, the connection and the account,
	// on the right.
	status       statusbar.Model
	hints, stats []statusbar.Item
	layers       []keyhelp.Layer
	bst          barStyles
	// keyhelp lists the keys that reach something while it is open.
	keyhelp keyhelp.Model
	// line is the command line, which takes the place of the status bar
	// while it is open.
	line cmdline.Model
	// going is the goto waiting for GitHub, or nil, and gotoSeq numbers
	// them, so that the answer to one replaced is ignored. spin shows in
	// the footer while one waits.
	going   *going
	gotoSeq int
	spin    spinner.Model
	theme   ui.Theme
	// icons are the set ui.icons names, which the root draws its marks,
	// frames and rules with.
	icons ui.Icons
	st    styles
	// header is rendered whenever what it shows changes.
	header string

	width, height int
	// links keeps the links drawn on every frame, such as a modal's title.
	links termtext.Links

	sync      func(ctx context.Context) (ui.SyncMsg, bool)
	setActive func(active bool)
	open      Browser
	watchRepo func(repo core.RepoRef)
	repoInfo  func(ctx context.Context, repo core.RepoRef) (core.Repo, error)
	// repos checks that a repository exists before goto opens it, and
	// kinds tells an issue from a pull request. host is the one whose
	// links goto opens.
	repos Repos
	kinds Kinds
	host  string
	// owners checks that a user or organization exists before goto
	// opens its page, and ownerLogin is the login of the page on view,
	// for the header.
	owners     Owners
	ownerLogin string
	// recall is what the command line completes from, with recent, the
	// repositories selected in this session, the latest first.
	recall Recall
	recent []core.RepoRef
	// recentOwners are the logins of the pages opened in this session,
	// the latest first.
	recentOwners []string
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
	// rates tells the rate limits, and rate is what it told last.
	// offSince is when the connection went offline, or zero while it
	// isn't, and failSince the start of the failing last logged, or zero
	// once it mended.
	rates     RateLimits
	rate      core.RateStatus
	offSince  time.Time
	failSince time.Time
	// online is told when GitHub answers again, as the sections are.
	// wokeAt is when they last were, and waking is set while a wake that
	// came too soon after waits for its turn.
	online func()
	wokeAt time.Time
	waking bool
	// login is the account's, for the status bar.
	login string
	// voice words what went wrong in the app's toasts and the modals it
	// opens.
	voice ui.Voice
	// settings is told the config when the set command changes it.
	settings func(config.Config)
	// access is what the app knows of the token, or nil, token checks
	// with it, and accessChanges tells its changes. noticed is set once
	// the user was told what the token can't do, and exec runs what
	// grants it more.
	access        Access
	token         *ui.Token
	accessChanges <-chan core.Access
	noticed       bool
	exec          runArgs
	// oldEnterprise tells of a GitHub Enterprise Server older than
	// supported.
	oldEnterprise <-chan string
	// lateWarning tells of a warning found once the app runs.
	lateWarning <-chan string
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

// Browser opens url in the browser, detached from the terminal. For a
// browser that runs in the terminal it starts nothing and returns the
// command, which the app runs with the terminal handed over.
type Browser func(url string) (*exec.Cmd, error)

// WithBrowser sets the function that opens a URL in the browser.
func WithBrowser(open Browser) Option {
	return func(m *Model) { m.open = open }
}

// WithVoice sets how the app words what went wrong, with the keys a hint
// names and the log it points to. By default the hints name the configured
// keys and no log.
func WithVoice(v ui.Voice) Option {
	return func(m *Model) { m.voice = v }
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
	keys := newKeyMap(cfg.Keys)
	m := &Model{
		ctx:        ctx,
		cfg:        cfg,
		file:       cfg,
		keys:       keys,
		toast:      toast.New(cfg.UI.Toast.Info, cfg.UI.Toast.Error),
		toastTimes: cfg.UI.Toast,
		keyhelp:    newHelp(keys),
		status:     statusbar.New(),
		line:       newLine(cfg.Keys, cfg.Commands.History),
		spin:       newSpinner(),
		voice:      ui.NewVoice(cfg.Keys, ""),
		images:     newImageProbe(cfg.Images.Enabled),
		after:      tick,
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
	if layout.Owner != nil {
		m.own = &pane{section: layout.Owner, bare: true}
		m.all = append(m.all, m.own)
	}
	for _, opt := range opts {
		opt(m)
	}
	m.voice.Icons = &m.icons
	m.readRates()
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

// Init asks for the terminal background and version, starts the sections on screen and
// the notifications, whose badge is on every screen, and listens for sync
// events.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.RequestBackgroundColor, waitTerminal(), m.pending, m.startScreen(), m.listen(), m.loadRepoInfo(), m.loadHistory(), m.startAccess(), m.listenOldEnterprise(), m.listenLateWarning()}
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
	case ownerScreen:
		return m.own.start()
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
	m.icons = ui.NewIcons(m.cfg.UI.Icons)
	m.voice.Icons = &m.icons
	m.st = newStyles(m.theme)
	m.toast.SetStyles(m.theme.Toast(m.icons))
	m.keyhelp.SetStyles(m.theme.KeyHelp(m.icons))
	m.bst = newBarStyles(m.theme)
	m.status.SetStyles(statusbar.Styles{Separator: m.st.edge, SeparatorText: m.icons.Separator})
	m.drawStatus()
	// The hints are drawn again in the theme's styles.
	m.layers = nil
	m.line.SetStyles(m.theme.Cmdline(m.icons))
	m.spin.Style = m.theme.Accent
	m.spin.Spinner = m.icons.SpinnerOr(spinner.MiniDot)
	for _, p := range m.all {
		p.section.SetTheme(m.theme)
	}
	if m.modal != nil {
		m.modal.SetTheme(m.theme)
	}
	m.drawFrames()
	m.drawHeader()
}

// updateBadges takes the badge of the notifications and the login of the
// owner page, for the header, and the chips of the panes, for their
// titles.
func (m *Model) updateBadges() {
	m.updateChips()
	m.updateOwner()
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

// OwnerPage is the section of the page of a user or an organization,
// which names the account it shows.
type OwnerPage interface {
	// Login returns the login of the account on view, or "" before one
	// is.
	Login() string
}

// updateOwner takes the login of the page of an owner, which changes as
// it goes back through the pages it showed, for the header.
func (m *Model) updateOwner() {
	if m.own == nil {
		return
	}
	o, ok := m.own.section.(OwnerPage)
	if !ok {
		return
	}
	if login := o.Login(); login != m.ownerLogin {
		m.ownerLogin = login
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
			label += m.icons.Separator + chips
		}
		if label != p.label {
			p.label = label
			m.drawFrame(p)
		}
	}
}
