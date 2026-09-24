package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Update routes msg: keys to the top modal, or else to the app or the
// focused pane, app messages to the app, and everything else to every
// section and modal.
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
		if msg.err == nil && msg.repo.Ref == m.repo {
			m.branch = msg.repo.DefaultBranch
			m.drawHeader()
		}
		return m, nil
	case ui.ShowMsg:
		cmd := m.show(msg.Title)
		return m, cmd
	case ui.OpenMsg:
		cmd := m.openURL(msg.URL)
		return m, cmd
	case ui.BackMsg:
		cmd := m.showScreen(m.back, m.focus)
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
	if p != nil && msg.String() != "ctrl+c" {
		c, capturer := p.section.(ui.Capturer)
		cl, claimer := p.section.(ui.Claimer)
		if capturer && c.Capturing() || claimer && cl.Claims(msg) {
			cmd := p.section.Update(msg)
			m.updateBadges()
			return cmd
		}
	}
	switch {
	case key.Matches(msg, m.keys.Quit):
		return tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.layout()
		return nil
	case key.Matches(msg, m.keys.Search):
		if m.srch != nil {
			return m.showSearch()
		}
		return m.openSearch()
	case m.canOpenHistory() && key.Matches(msg, m.keys.History):
		return m.openHistory()
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

// showSearch shows the search page, with the focus in its query.
func (m *Model) showSearch() tea.Cmd {
	if m.screen == searchScreen {
		m.srch.setFocus(true)
		return nil
	}
	return m.showScreen(searchScreen, m.focus)
}

// selectRepo shows the repository screen for the repository of msg, with
// the files focused, after telling the watcher and the sections.
func (m *Model) selectRepo(msg ui.RepoMsg) tea.Cmd {
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
	// The closed search still gets its own messages, such as the spinner
	// tick, so it is not stuck when it opens again.
	if m.searchBox != nil && !m.isOpen(m.searchBox) {
		cmds = append(cmds, m.searchBox.Update(msg))
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
