package tui

import (
	"log/slog"
	"os"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Update routes msg: keys to the open command line, or else to the open
// help, or else to the top modal, or else to the app or the focused pane,
// app messages to the app, and everything else to every section and
// modal. The images that what it drew asks for are fetched after it.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.update(msg)
	if load := m.loadImages(); load != nil {
		cmd = tea.Batch(cmd, load)
	}
	return m, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.term.observe(m.ctx, msg)
	// The answers of the terminal to the cell query and the images probe
	// are theirs. The query goes first, since it takes a DA1 only while it
	// asks, and the probe takes every other.
	if cmd, handled := m.cells.update(msg); handled {
		return m, cmd
	}
	if cmd, handled := m.images.update(msg); handled {
		return m, cmd
	}
	if cmd, redraw, handled := m.pics.Update(msg); handled {
		return m, tea.Batch(cmd, m.redrawImages(redraw))
	}
	switch msg := msg.(type) {
	case terminalWaitMsg:
		return m, nil
	case imagesDueMsg:
		if !m.picsDue {
			return m, nil
		}
		m.picsDue = false
		cmd := m.imagesChanged()
		return m, cmd
	case imagesClearMsg:
		cmd := m.clearImages(msg)
		return m, cmd
	case imagesClearedMsg:
		cmd := m.imagesCleared(msg)
		return m, cmd
	case tmuxMovedMsg:
		// tmux is attached from another terminal, which has none of the
		// images sent to the one before.
		return m, m.pics.Resend()
	case tea.ColorProfileMsg:
		return m, tea.Batch(m.images.plan(m.ctx, msg.Profile), m.broadcast(msg))
	case graphicsDecidedMsg:
		cmd := m.graphicsDecided(msg)
		return m, cmd
	case cellSizedMsg:
		cmd := m.cellSized(msg)
		return m, cmd
	case cellsAgainMsg:
		cmd := m.askCells()
		return m, cmd
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		m.cells.resize(msg.Width, msg.Height)
		// A resize may come of a change of font, which changes the cells.
		cmd := tea.Batch(m.askCells(), m.settleModal())
		return m, cmd
	case tea.BackgroundColorMsg:
		m.applyTheme(msg.IsDark())
		return m, nil
	case tea.FocusMsg:
		m.report(true)
		cmd := tea.Batch(m.images.askClient(m.ctx), m.pics.Show())
		return m, cmd
	case tea.BlurMsg:
		m.report(false)
		m.hideImages()
		return m, nil
	case tea.KeyPressMsg:
		cmd := m.key(msg)
		return m, cmd
	case tea.PasteMsg:
		// A link pasted into the open command line goes there, and text
		// pasted into the open help's query there.
		switch {
		case m.line.Focused():
			cmd := m.updateLine(msg)
			return m, cmd
		case m.helpOpen():
			cmd := m.updateHelp(msg)
			return m, cmd
		}
	case keyhelp.CloseMsg:
		if msg.ID == m.keyhelp.ID() {
			m.keyhelp.Blur()
			return m, nil
		}
	case cmdline.SubmitMsg:
		if msg.ID == m.line.ID() {
			m.lineDone()
			cmd := m.runLine(msg.Line, m.saveHistory())
			return m, cmd
		}
	case historyMsg:
		cmd := m.loadedHistory(msg)
		return m, cmd
	case accessChangedMsg, authRanMsg, authReloadedMsg:
		cmd := m.updateAccess(msg)
		return m, cmd
	case oldEnterpriseMsg:
		cmd := m.toldOldEnterprise(msg)
		return m, cmd
	case lateWarningMsg:
		cmd := m.toast.Push(toast.Warning, msg.text)
		return m, cmd
	case quitWaitedMsg:
		cmd := m.quitWaited()
		return m, cmd
	case gotoRepoMsg:
		cmd := m.gotRepo(msg)
		return m, cmd
	case gotoKindMsg:
		cmd := m.gotKind(msg)
		return m, cmd
	case gotoOwnerMsg:
		cmd := m.gotOwner(msg)
		return m, cmd
	case ui.OwnerMsg:
		cmd := m.showOwner(msg.Login)
		return m, cmd
	case spinner.TickMsg:
		if msg.ID == m.spin.ID() {
			if m.going == nil {
				return m, nil
			}
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
	case cmdline.CancelMsg:
		if msg.ID == m.line.ID() {
			m.lineDone()
			return m, nil
		}
	case ui.NotifyMsg:
		return m, m.toast.Push(msg.Level, msg.Text)
	case ui.FailMsg:
		cmd := m.fail(msg.What, msg.Err)
		return m, cmd
	case ui.DoneMsg:
		cmd := m.fail(msg.What, msg.Err)
		return m, tea.Batch(cmd, m.broadcast(msg))
	case ui.SyncMsg:
		if msg.Key == core.SyncRateLimit {
			// Only the status bar shows the rate limits, but GitHub
			// answering again wakes what failed.
			cmd := m.readRates()
			return m, tea.Batch(cmd, m.listen())
		}
		return m, tea.Batch(m.broadcast(msg), m.listen())
	case onlineTickMsg:
		cmd := m.onlineTick()
		return m, cmd
	case ui.RepoMsg:
		cmd := m.selectRepo(msg)
		return m, cmd
	case ui.BaseMsg:
		if !msg.Repo.Same(m.repo) {
			return m, nil
		}
		m.base = msg
		m.drawHeader()
		cmd := m.broadcast(msg)
		return m, cmd
	case repoInfoMsg:
		if msg.err != nil || !msg.repo.Ref.Same(m.repo) {
			return m, nil
		}
		m.branch, m.ownerAvatar = msg.repo.DefaultBranch, msg.repo.OwnerAvatarURL
		m.drawHeader()
		// The sections and the modal gate their changes on the caps.
		cmd := m.broadcast(ui.CapsMsg{Repo: m.repo, Caps: msg.repo.Caps})
		return m, cmd
	case ui.ShowMsg:
		cmd := m.show(msg.Title)
		return m, cmd
	case ui.OpenMsg:
		cmd := m.openURL(msg.URL)
		return m, cmd
	case ui.BackMsg:
		cmd := m.showScreen(m.back, m.focus)
		return m, cmd
	case ui.OpenActionsMsg:
		cmd := m.openActionsOn(msg.Repo, msg.Filter)
		return m, cmd
	case ui.OpenCommitMsg:
		cmd := m.openCommit(msg)
		return m, cmd
	case ui.OpenReleaseMsg:
		cmd := m.openRelease(msg)
		return m, cmd
	case ui.OpenPullMsg:
		// Away from the repository screen, a number alone doesn't say
		// which repository it is of.
		msg.ShowRepo = msg.ShowRepo || m.screen != repoScreen
		cmd := m.broadcast(msg)
		return m, cmd
	case ui.OpenIssueMsg:
		msg.ShowRepo = msg.ShowRepo || m.screen != repoScreen
		cmd := m.broadcast(msg)
		return m, cmd
	case ui.OpenModalMsg:
		m.openModal(msg.Modal)
		return m, nil
	case ui.CloseModalMsg:
		m.closeModal(msg.Modal)
		return m, nil
	}

	var cmd tea.Cmd
	m.toast, cmd = m.toast.Update(msg)
	return m, tea.Batch(cmd, m.broadcast(msg))
}

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	// The open command line takes every key, ctrl+c too, which cancels
	// the command rather than quitting.
	if m.line.Focused() {
		return m.updateLine(msg)
	}
	// ctrl+c quits from the help and a modal, which take every other
	// key, so that they can't trap the user.
	if m.helpOpen() {
		if key.Matches(msg, forceQuit) {
			return tea.Quit
		}
		return m.updateHelp(msg)
	}
	// The help key reaches the app from inside a modal and a capturing
	// section, unless it types into an input there.
	if m.opensHelp(msg) {
		return m.openHelp()
	}
	if mod := m.topModal(); mod != nil {
		if key.Matches(msg, forceQuit) {
			return tea.Quit
		}
		if m.commandsOver(mod) && key.Matches(msg, m.keys.Command) {
			return m.openLine()
		}
		cmd := mod.Update(msg)
		m.updateBadges()
		return cmd
	}
	p := m.focused()
	// ctrl+c always reaches the app's keys, so a capturing section can't
	// trap the user.
	if p != nil && !key.Matches(msg, forceQuit) && m.takes(p.section, msg) {
		cmd := p.section.Update(msg)
		m.updateBadges()
		return cmd
	}
	switch {
	case key.Matches(msg, m.keys.Command):
		return m.openLine()
	case key.Matches(msg, m.keys.Quit):
		return tea.Quit
	case key.Matches(msg, m.keys.Search):
		return m.showSearch()
	case m.canOpenHistory() && key.Matches(msg, m.keys.History):
		return m.openHistory()
	case m.canOpenActions() && key.Matches(msg, m.keys.Actions):
		return m.openActions()
	case m.fileFinder() != nil && key.Matches(msg, m.keys.FindFile):
		return m.findFile()
	case p != nil && key.Matches(msg, m.keys.Filter) && m.openFilter(p.section, filterform.FiltersTab):
		return nil
	case p != nil && key.Matches(msg, m.keys.Sort) && m.openFilter(p.section, filterform.SortTab):
		return nil
	case m.canZoom() && m.width >= narrowWidth && key.Matches(msg, m.keys.Zoom):
		m.setZoom(!m.zoom)
		return nil
	case m.canZoom() && m.zoomed() && key.Matches(msg, m.keys.Back):
		m.setZoom(false)
		return nil
	case key.Matches(msg, m.toast.KeyMap().Dismiss):
		return m.toast.Dismiss()
	case key.Matches(msg, m.keys.Owner) && m.selectedOwner() != "":
		return m.showOwner(m.selectedOwner())
	case key.Matches(msg, m.keys.Notifications):
		return m.toggleScreen(notifScreen)
	case m.dash != nil && key.Matches(msg, m.keys.Dashboard):
		return m.toggleScreen(dashScreen)
	}
	// Only the repository screen has panes for the app to cycle through;
	// elsewhere these keys are the section's.
	if m.screen == repoScreen {
		switch {
		case key.Matches(msg, m.keys.Next):
			return m.cycle(1)
		case key.Matches(msg, m.keys.Prev):
			return m.cycle(-1)
		}
	}
	// The dashboard and the owner page move between their own panes.
	if m.screen != dashScreen && m.screen != ownerScreen {
		if i := m.keys.pane(msg); i >= 0 && i < len(m.panes) {
			return m.showScreen(repoScreen, i)
		}
	}
	if p == nil {
		return nil
	}
	cmd := p.section.Update(msg)
	m.updateBadges()
	return cmd
}

// takes reports whether s takes msg before the app: while it captures
// every key, or when it claims msg.
func (m *Model) takes(s ui.Section, msg tea.KeyPressMsg) bool {
	if c, ok := s.(ui.Capturer); ok && c.Capturing() {
		return true
	}
	c, ok := s.(ui.Claimer)
	return ok && key.Matches(msg, c.Claimed()...)
}

// openFilter opens the filter modal of s on tab, and reports whether s has
// one to open. The key goes on to a section that has none, which may use
// it otherwise.
func (m *Model) openFilter(s ui.Section, tab filterform.Tab) bool {
	fl, f, ok := filterOf(s, tab)
	if !ok {
		return false
	}
	m.openModal(ui.NewFilterModal(m.ctx, s.Title(), fl, f, ui.OnTab(tab), ui.WithFormKeys(m.keys.form), ui.WithFormVoice(m.voice),
		ui.WithFormIcons(ui.NewIcons(m.cfg.UI.Icons))))
	return true
}

// filterOf returns what the filter modal edits for s on tab, if s has one
// to open there: the Sort tab needs a list that can be sorted.
func filterOf(s ui.Section, tab filterform.Tab) (ui.Filterable, ui.Filter, bool) {
	fl, ok := s.(ui.Filterable)
	if !ok {
		return nil, ui.Filter{}, false
	}
	f, ok := fl.Filter()
	if !ok || tab == filterform.SortTab && f.Spec.Sort == nil {
		return nil, ui.Filter{}, false
	}
	return fl, f, true
}

// showSearch shows the search page, with the focus in an empty query, if
// the app has one. The page keeps its recent searches, but not the last
// query, so typing starts a new one.
func (m *Model) showSearch() tea.Cmd {
	if m.srch == nil {
		return nil
	}
	if f, ok := m.srch.section.(Fresher); ok {
		f.Fresh()
	}
	if m.screen == searchScreen {
		m.srch.setFocus(true)
		return nil
	}
	return m.showScreen(searchScreen, m.focus)
}

// selectRepo shows the repository screen for the repository of msg, with
// the files focused, after telling the watcher and the sections.
func (m *Model) selectRepo(msg ui.RepoMsg) tea.Cmd {
	m.cancelGoto()
	m.remember(msg.Repo)
	if m.watchRepo != nil {
		m.watchRepo(msg.Repo)
	}
	if !msg.Repo.Same(m.repo) {
		m.repo, m.branch, m.ownerAvatar = msg.Repo, "", ""
	}
	// Selecting a repository shows the head of its default branch.
	m.base = ui.BaseMsg{}
	m.drawHeader()
	return tea.Batch(m.broadcast(msg), m.showScreen(repoScreen, 0), m.loadRepoInfo())
}

// broadcast sends msg to every section, started or not, so that a section
// shown later already knows, for example, which repository was selected,
// and to the open modal.
func (m *Model) broadcast(msg tea.Msg) tea.Cmd {
	panes := m.all
	cmds := make([]tea.Cmd, 0, len(panes)+2)
	for _, p := range panes {
		cmds = append(cmds, p.section.Update(msg))
	}
	if m.modal != nil {
		cmds = append(cmds, m.modal.Update(msg))
	}
	m.updateBadges()
	return tea.Batch(cmds...)
}

// show shows the section titled title, on its screen.
func (m *Model) show(title string) tea.Cmd {
	for i, p := range m.panes {
		if p.section.Title() == title {
			return m.showScreen(repoScreen, i)
		}
	}
	if m.notif != nil && m.notif.section.Title() == title {
		return m.showScreen(notifScreen, m.focus)
	}
	for s, p := range map[screen]*pane{dashScreen: m.dash, searchScreen: m.srch, ownerScreen: m.own} {
		if p != nil && p.section.Title() == title {
			return m.showScreen(s, m.focus)
		}
	}
	return nil
}

// browserFix says what to change when the browser didn't open. gh
// takes GH_BROWSER before its config's browser, so while GH_BROWSER is
// set the config changes nothing.
func browserFix() string {
	if os.Getenv("GH_BROWSER") != "" {
		return "GH_BROWSER names a command that didn't open it."
	}
	return "set one with gh config set browser <command>."
}

func (m *Model) openURL(url string) tea.Cmd {
	if m.open == nil || url == "" {
		return nil
	}
	open, ctx := m.open, m.ctx
	return func() tea.Msg {
		if err := open(url); err != nil {
			slog.WarnContext(ctx, "browser failed", "span", "tui", "err", err.Error())
			return ui.NotifyMsg{Level: toast.Error, Text: "Couldn't open the browser: " + browserFix()}
		}
		return nil
	}
}

func (m *Model) report(active bool) {
	if m.setActive != nil {
		m.setActive(active)
	}
}

// fail tells in a toast what stopped action, if anything did and the user
// should hear of it.
func (m *Model) fail(action string, err error) tea.Cmd {
	text := ui.SayToast(core.Explain(action, err), m.voice, m.fitsToast)
	if text == "" {
		return nil
	}
	return m.toast.Push(toast.Error, text)
}

// fitsToast reports whether an error toast shows text whole.
func (m *Model) fitsToast(text string) bool { return m.toast.Fits(toast.Error, text) }
