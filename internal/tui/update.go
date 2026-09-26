package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Update routes msg: keys to the open command line, or else to the top
// modal, or else to the app or the focused pane, app messages to the
// app, and everything else to every section and modal.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil
	case tea.BackgroundColorMsg:
		m.applyTheme(msg.IsDark())
		return m, nil
	case tea.FocusMsg:
		m.report(true)
		return m, nil
	case tea.BlurMsg:
		m.report(false)
		return m, nil
	case tea.KeyPressMsg:
		cmd := m.key(msg)
		return m, cmd
	case tea.PasteMsg:
		// A link pasted into the open command line goes there.
		if m.line.Focused() {
			cmd := m.updateLine(msg)
			return m, cmd
		}
	case cmdline.SubmitMsg:
		if msg.ID == m.line.ID() {
			m.lineDone()
			cmd := m.runLine(msg.Line)
			return m, cmd
		}
	case gotoRepoMsg:
		cmd := m.gotRepo(msg)
		return m, cmd
	case gotoKindMsg:
		cmd := m.gotKind(msg)
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
	case ui.DoneMsg:
		var cmd tea.Cmd
		if msg.Err != nil {
			cmd = m.toast.Push(toast.Error, fmt.Sprintf("Couldn't %s: %v", msg.What, msg.Err))
		}
		return m, tea.Batch(cmd, m.broadcast(msg))
	case ui.SyncMsg:
		return m, tea.Batch(m.broadcast(msg), m.listen())
	case ui.RepoMsg:
		cmd := m.selectRepo(msg)
		return m, cmd
	case ui.BaseMsg:
		if !sameRepo(msg.Repo, m.repo) {
			return m, nil
		}
		m.base = msg
		m.drawHeader()
		cmd := m.broadcast(msg)
		return m, cmd
	case repoInfoMsg:
		if msg.err != nil || msg.repo.Ref != m.repo {
			return m, nil
		}
		m.branch = msg.repo.DefaultBranch
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
	if mod := m.topModal(); mod != nil {
		if msg.String() == "ctrl+c" {
			return tea.Quit
		}
		cmd := mod.Update(msg)
		m.updateBadges()
		return cmd
	}
	p := m.focused()
	// ctrl+c always reaches the quit key, so a capturing section can't
	// trap the user.
	if p != nil && msg.String() != "ctrl+c" && m.takes(p.section, msg) {
		cmd := p.section.Update(msg)
		m.updateBadges()
		return cmd
	}
	switch {
	case key.Matches(msg, m.keys.Command):
		return m.openLine()
	case key.Matches(msg, m.keys.Quit):
		return tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.layout()
		return nil
	case key.Matches(msg, m.keys.Search):
		return m.showSearch()
	case m.canOpenHistory() && key.Matches(msg, m.keys.History):
		return m.openHistory()
	case m.canOpenActions() && key.Matches(msg, m.keys.Actions):
		return m.openActions()
	case m.fileFinder() != nil && key.Matches(msg, m.keys.FindFile):
		return m.findFile()
	case p != nil && key.Matches(msg, m.keys.Filter) && m.openFilter(p.section):
		return nil
	case m.canZoom() && m.width >= narrowWidth && key.Matches(msg, m.keys.Zoom):
		m.setZoom(!m.zoom)
		return nil
	case m.canZoom() && m.zoomed() && key.Matches(msg, m.keys.Back):
		m.setZoom(false)
		return nil
	case key.Matches(msg, m.toast.KeyMap().Dismiss):
		return m.toast.Dismiss()
	case key.Matches(msg, m.keys.Notifications):
		return m.toggleScreen(notifScreen)
	case m.dash != nil && key.Matches(msg, m.keys.Dashboard):
		return m.toggleScreen(dashScreen)
	}
	// The dashboard moves between its own panes.
	if m.screen != dashScreen {
		switch {
		case key.Matches(msg, m.keys.Next):
			return m.cycle(1)
		case key.Matches(msg, m.keys.Prev):
			return m.cycle(-1)
		}
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
	return ok && c.Claims(msg)
}

// openFilter opens the filter modal of s, and reports whether s has one to
// open. The filter key goes on to a section that doesn't, which may use it
// otherwise.
func (m *Model) openFilter(s ui.Section) bool {
	fl, ok := s.(ui.Filterable)
	if !ok {
		return false
	}
	f, ok := fl.Filter()
	if !ok {
		return false
	}
	m.openModal(ui.NewFilterModal(m.ctx, s.Title(), fl, f))
	return true
}

// showSearch shows the search page, with the focus in its query, if the
// app has one.
func (m *Model) showSearch() tea.Cmd {
	if m.srch == nil {
		return nil
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
	if m.watchRepo != nil {
		m.watchRepo(msg.Repo)
	}
	if msg.Repo != m.repo {
		m.repo, m.branch = msg.Repo, ""
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
	for s, p := range map[screen]*pane{dashScreen: m.dash, searchScreen: m.srch} {
		if p != nil && p.section.Title() == title {
			return m.showScreen(s, m.focus)
		}
	}
	return nil
}

func (m *Model) openURL(url string) tea.Cmd {
	if m.open == nil || url == "" {
		return nil
	}
	open := m.open
	return func() tea.Msg {
		if err := open(url); err != nil {
			return ui.NotifyMsg{Level: toast.Error, Text: "Couldn't open the browser: " + err.Error()}
		}
		return nil
	}
}

// sameRepo reports whether a and b name the same repository, which GitHub
// matches regardless of case.
func sameRepo(a, b core.RepoRef) bool {
	return strings.EqualFold(a.Owner, b.Owner) && strings.EqualFold(a.Name, b.Name)
}

func (m *Model) report(active bool) {
	if m.setActive != nil {
		m.setActive(active)
	}
}
