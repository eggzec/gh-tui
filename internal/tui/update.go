package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/tabs"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Update routes msg: keys to the top modal, or else to the tabs or the
// active section, app messages to the app, and everything else to every
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
	case tabs.ChangeMsg:
		if msg.ID != m.tabs.ID() {
			return m, nil
		}
		cmd := m.switchTo(msg.Index)
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
		if m.watchRepo != nil {
			m.watchRepo(msg.Repo)
		}
		cmd := m.broadcast(msg)
		return m, cmd
	case ui.ShowMsg:
		cmd := m.show(msg.Title)
		return m, cmd
	case ui.OpenMsg:
		cmd := m.openURL(msg.URL)
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
	// ctrl+c always reaches the quit key, so a capturing section can't
	// trap the user.
	if c, ok := m.activeSection().(ui.Capturer); ok && c.Capturing() && msg.String() != "ctrl+c" {
		cmd := m.sections[m.active].Update(msg)
		m.updateBadges()
		return cmd
	}
	tk := m.tabs.KeyMap()
	switch {
	case key.Matches(msg, m.keys.Quit):
		return tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.layout()
		return nil
	case key.Matches(msg, m.keys.Search):
		return m.openSearch()
	case key.Matches(msg, m.toast.KeyMap().Dismiss):
		return m.toast.Dismiss()
	case key.Matches(msg, tk.Next, tk.Prev, tk.Jump):
		var cmd tea.Cmd
		m.tabs, cmd = m.tabs.Update(msg)
		return cmd
	}
	if len(m.sections) == 0 {
		return nil
	}
	cmd := m.sections[m.active].Update(msg)
	m.updateBadges()
	return cmd
}

// activeSection returns the section on screen, or nil if there are none.
func (m *Model) activeSection() ui.Section {
	if len(m.sections) == 0 {
		return nil
	}
	return m.sections[m.active]
}

// broadcast sends msg to every section, started or not, so that a section
// shown later already knows, for example, which repository was selected,
// and to every open modal.
func (m *Model) broadcast(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.sections)+len(m.modals))
	for _, s := range m.sections {
		cmds = append(cmds, s.Update(msg))
	}
	for _, mod := range m.modals {
		cmds = append(cmds, mod.Update(msg))
	}
	// The closed search still gets its own messages, such as the spinner
	// tick, so it is not stuck when it opens again.
	if m.searchBox != nil && !m.isOpen(m.searchBox) {
		cmds = append(cmds, m.searchBox.Update(msg))
	}
	m.updateBadges()
	return tea.Batch(cmds...)
}

func (m *Model) switchTo(i int) tea.Cmd {
	if i < 0 || i >= len(m.sections) || i == m.active {
		return nil
	}
	m.sections[m.active].Blur()
	m.active = i
	m.sections[i].Focus()
	return m.start(i)
}

func (m *Model) show(title string) tea.Cmd {
	for i, s := range m.sections {
		if s.Title() == title {
			return m.tabs.SetActive(i)
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

func (m *Model) report(active bool) {
	if m.setActive != nil {
		m.setActive(active)
	}
}
