package tui

import (
	"log/slog"
	"os"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Update routes msg: keys to the open command line, or else to the open
// help, or else to the top modal, or else to the app or the focused pane,
// app messages to the app, and everything else to every section and
// modal. The open modal waits out what of it resized it, and the images
// that what it drew asks for are fetched after it.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.update(msg)
	m.fitModal()
	if m.resized {
		m.resized = false
		cmd = tea.Batch(cmd, m.settleModal())
	}
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
		cmd := m.askCells()
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
		cmd := m.gotoOwner(core.Target{Owner: msg.Login})
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
	case starDoneMsg:
		cmd := m.starDone(msg)
		return m, cmd
	case ui.BulkDoneMsg:
		var cmd tea.Cmd
		if level, text := msg.Toast(m.voice, m.toast.Fits); text != "" {
			cmd = m.toast.Push(level, text)
		}
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
	case ui.OpenFilterMsg:
		// The focused list asked, with its own filter or sort key.
		if p := m.focused(); p != nil {
			m.openFilter(p.section, msg.Tab)
		}
		return m, nil
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
		m.openModalOver(msg.Modal, msg.Back)
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
	// The dismiss key goes one step at a time, before anything else takes
	// it, even the command line and the help, and a modal: an error toast
	// first, and then a goto that waits for GitHub. What is left is the
	// line's, the help's, the modal's or the section's to clear and then
	// close.
	if keymap.Matches(msg, m.keys.Dismiss) {
		switch {
		case m.toast.Has(toast.Error):
			m.toast.DismissLevel(toast.Error)
			return nil
		case m.going != nil:
			m.cancelGoto()
			return nil
		}
	}
	// The open command line takes every key but ctrl+c, which quits from
	// everywhere; esc cancels the line.
	if m.line.Focused() {
		if keymap.Matches(msg, forceQuit) {
			return tea.Quit
		}
		return m.updateLine(msg)
	}
	// ctrl+c quits from the help and a modal, which take every other
	// key, so that they can't trap the user.
	if m.helpOpen() {
		if keymap.Matches(msg, forceQuit) {
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
		if keymap.Matches(msg, forceQuit) {
			return tea.Quit
		}
		if m.commandsOver() && keymap.Matches(msg, m.keys.Command) {
			return m.openLine()
		}
		takes := m.modalTakesKeys()
		if !takes && keymap.Matches(msg, m.keys.Maximize) {
			m.toggleMaximized()
			return nil
		}
		if !takes {
			if cmd, handled := m.actOverModal(mod, msg); handled {
				return cmd
			}
		}
		cmd := mod.Update(msg)
		m.updateBadges()
		return cmd
	}
	p := m.focused()
	// ctrl+c always reaches the app's keys, so a capturing section can't
	// trap the user.
	if p != nil && !keymap.Matches(msg, forceQuit) && m.takes(p.section) {
		cmd := p.section.Update(msg)
		m.updateBadges()
		return cmd
	}
	switch {
	case keymap.Matches(msg, m.keys.Command):
		return m.openLine()
	case keymap.Matches(msg, m.keys.Quit):
		return tea.Quit
	case keymap.Matches(msg, m.keys.Search):
		return m.showSearch()
	case m.canOpenHistory() && keymap.Matches(msg, m.keys.History):
		return m.openHistory()
	case m.canOpenActions() && keymap.Matches(msg, m.keys.Actions):
		return m.openActions()
	case m.canStar() && keymap.Matches(msg, m.keys.Star):
		return m.star()
	case m.fileFinder() != nil && keymap.Matches(msg, m.keys.FindFile):
		return m.findFile()
	case m.canZoom() && m.width >= narrowWidth && keymap.Matches(msg, m.keys.Zoom):
		m.setZoom(!m.zoom)
		return nil
	case keymap.Matches(msg, m.keys.Back):
		return m.goBack()
	case keymap.Matches(msg, m.keys.Owner):
		// Without an owner, the key goes on to the section.
		if owner := m.selectedOwner(); owner != "" {
			return m.gotoOwner(core.Target{Owner: owner})
		}
	case keymap.Matches(msg, m.keys.Repo):
		if repo := m.selectedRepo(); repo != (core.RepoRef{}) {
			if p := m.focused(); p != nil {
				if o, ok := p.section.(ui.RepoOpener); ok {
					sel, _ := m.selection()
					o.OpenedRepo(sel)
				}
			}
			return m.selectRepo(ui.RepoMsg{Repo: repo})
		}
	case keymap.Matches(msg, m.keys.Notifications):
		return m.showScreen(notifScreen, m.focus)
	case m.dash != nil && keymap.Matches(msg, m.keys.Dashboard):
		return m.showScreen(dashScreen, m.focus)
	}
	// Only the repository screen has panes for the app to cycle through;
	// elsewhere these keys are the section's.
	if m.screen == repoScreen {
		switch {
		case keymap.Matches(msg, m.keys.Next):
			return m.cycle(1)
		case keymap.Matches(msg, m.keys.Prev):
			return m.cycle(-1)
		}
	}
	// The other screens focus their own panes, or ignore the digit.
	if m.screen == repoScreen {
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
// every key.
func (m *Model) takes(s ui.Section) bool {
	c, ok := s.(ui.Capturer)
	return ok && c.Capturing()
}

// openFilter opens the filter modal of s on tab, and reports whether s has
// one to open. The key goes on to a section that has none, which may use
// it otherwise.
func (m *Model) openFilter(s ui.Section, tab filterform.Tab) bool {
	fl, f, ok := filterOf(s, tab)
	if !ok {
		return false
	}
	m.openModal(ui.NewFilterModal(m.ctx, s.Title(), fl, f, ui.OnTab(tab), ui.WithFormKeys(m.keys.form), ui.WithFormBack(m.keys.Back), ui.WithFormVoice(m.voice),
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
	if m.screen != repoScreen || !msg.Repo.Same(m.repo) {
		m.pushBack()
	}
	return m.openRepo(msg, 0)
}

// openRepo shows the repository of msg on the repository screen, with pane
// focus focused, without a way back to what was on view.
func (m *Model) openRepo(msg ui.RepoMsg, focus int) tea.Cmd {
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
	return tea.Batch(m.broadcast(msg), m.reveal(repoScreen, focus), m.loadRepoInfo())
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
	failed := func(err error) tea.Msg {
		if err == nil {
			return nil
		}
		slog.WarnContext(ctx, "browser failed", "span", "tui", "err", err.Error())
		return ui.NotifyMsg{Level: toast.Error, Text: "Couldn't open the browser: " + browserFix()}
	}
	return func() tea.Msg {
		cmd, err := open(url)
		if err != nil {
			return failed(err)
		}
		if cmd != nil {
			return tea.ExecProcess(cmd, failed)()
		}
		// A detached browser has no way onto the screen, but a redraw
		// costs one frame and mends it should anything have got there.
		return tea.ClearScreen()
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

// fitsWarning reports whether a warning toast shows text whole.
func (m *Model) fitsWarning(text string) bool { return m.toast.Fits(toast.Warning, text) }
