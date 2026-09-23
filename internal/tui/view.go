package tui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View lays out the tabs, the active section and the help line, with the
// toasts over them.
func (m *Model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.tabs.View())
	b.WriteByte('\n')
	if len(m.sections) > 0 {
		b.WriteString(m.sections[m.active].View())
	}
	b.WriteByte('\n')
	b.WriteString(m.help.View(m.helpKeys()))

	v := tea.NewView(m.toast.Overlay(b.String(), m.width, m.height))
	v.AltScreen = true
	v.ReportFocus = true
	v.WindowTitle = "gh-tui"
	return v
}

func (m *Model) helpHeight() int {
	return lipgloss.Height(m.help.View(m.helpKeys()))
}

func (m *Model) helpKeys() help.KeyMap {
	hk := helpKeys{app: m.keys, dismiss: m.toast.KeyMap().Dismiss}
	if len(m.sections) > 0 {
		hk.section = m.sections[m.active].Help()
	}
	return hk
}

// helpKeys lists the keys of the active section before the app's own.
type helpKeys struct {
	section help.KeyMap
	app     KeyMap
	dismiss key.Binding
}

func (h helpKeys) ShortHelp() []key.Binding {
	var ks []key.Binding
	if h.section != nil {
		ks = h.section.ShortHelp()
	}
	return append(ks, h.app.Help, h.app.Quit)
}

func (h helpKeys) FullHelp() [][]key.Binding {
	var groups [][]key.Binding
	if h.section != nil {
		groups = h.section.FullHelp()
	}
	return append(groups, []key.Binding{h.app.Tabs.Next, h.app.Tabs.Prev, h.dismiss, h.app.Help, h.app.Quit})
}
