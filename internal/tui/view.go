package tui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/overlay"
)

// View lays out the tabs, the active section and the help line, with the
// top modal and the toasts over them.
func (m *Model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.tabs.View())
	b.WriteByte('\n')
	if len(m.sections) > 0 {
		b.WriteString(m.sections[m.active].View())
	}
	b.WriteByte('\n')
	b.WriteString(m.help.View(m.helpKeys()))

	screen := b.String()
	if mod := m.topModal(); mod != nil {
		// The modal is centered on the terminal, so the screen under it
		// must fill it even while a section draws less.
		screen = lipgloss.PlaceVertical(m.height, lipgloss.Top, screen)
		screen = overlay.Center(screen, m.frame(mod), m.width, m.height)
	}
	v := tea.NewView(m.toast.Overlay(screen, m.width, m.height))
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
	if mod := m.topModal(); mod != nil {
		// The app's own keys don't reach a modal.
		hk.section, hk.modal = mod.Help(), true
		return hk
	}
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
	// modal hides the app's keys while a modal takes them.
	modal bool
}

func (h helpKeys) ShortHelp() []key.Binding {
	var ks []key.Binding
	if h.section != nil {
		ks = h.section.ShortHelp()
	}
	if h.modal {
		return ks
	}
	return append(ks, h.app.Search, h.app.Help, h.app.Quit)
}

func (h helpKeys) FullHelp() [][]key.Binding {
	var groups [][]key.Binding
	if h.section != nil {
		groups = h.section.FullHelp()
	}
	if h.modal {
		return groups
	}
	return append(groups, []key.Binding{h.app.Search, h.app.Tabs.Next, h.app.Tabs.Prev, h.dismiss, h.app.Help, h.app.Quit})
}
