package tui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/overlay"
)

// View lays out the header, the screen on view and the help line, with the
// top modal and the toasts over them.
func (m *Model) View() tea.View {
	body := m.body()
	var b strings.Builder
	n := len(m.header) + 1 + len(body)
	for _, l := range body {
		n += len(l)
	}
	b.Grow(n + 256)
	b.WriteString(m.header)
	for _, l := range body {
		b.WriteByte('\n')
		b.WriteString(l)
	}
	// The screen fills its height even when it has nothing to show, so the
	// help line stays at the bottom.
	for range m.contentHeight() - len(body) {
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(m.help.View(m.helpKeys()))

	screen := b.String()
	if mod := m.topModal(); mod != nil {
		// The modal is centered on the terminal, so the screen under it
		// must fill it.
		screen = lipgloss.PlaceVertical(m.height, lipgloss.Top, screen)
		screen = overlay.Center(screen, m.frame(mod), m.width, m.height)
	}
	v := tea.NewView(m.toast.Overlay(screen, m.width, m.height))
	v.AltScreen = true
	v.ReportFocus = true
	v.WindowTitle = "gh-tui"
	return v
}

// layout gives each part its share of the screen.
func (m *Model) layout() {
	m.help.SetWidth(m.width)
	m.toast.SetSize(m.width, m.height)
	m.arrange(m.contentHeight())
	for _, mod := range m.modals {
		mod.SetSize(m.modalSize())
	}
	m.drawFrames()
	m.drawHeader()
}

// contentHeight is the height between the header and the help line.
func (m *Model) contentHeight() int {
	return max(m.height-1-m.helpHeight(), 0)
}

func (m *Model) helpHeight() int {
	return lipgloss.Height(m.help.View(m.helpKeys()))
}

func (m *Model) helpKeys() help.KeyMap {
	hk := helpKeys{app: m.keys, dismiss: m.toast.KeyMap().Dismiss, notifications: m.keys.Notifications}
	if m.screen == notifScreen {
		hk.notifications.SetHelp(hk.notifications.Help().Key, "back")
	}
	if mod := m.topModal(); mod != nil {
		// The app's own keys don't reach a modal.
		hk.section, hk.modal = mod.Help(), true
		return hk
	}
	if p := m.focused(); p != nil {
		hk.section = p.section.Help()
	}
	hk.panes = m.screen == repoScreen && len(m.panes) > 1
	return hk
}

// helpKeys lists the keys of the focused pane before the app's own.
type helpKeys struct {
	section       help.KeyMap
	app           KeyMap
	dismiss       key.Binding
	notifications key.Binding
	// modal hides the app's keys while a modal takes them.
	modal bool
	// panes shows the keys that move between panes.
	panes bool
}

func (h helpKeys) ShortHelp() []key.Binding {
	var ks []key.Binding
	if h.section != nil {
		ks = h.section.ShortHelp()
	}
	if h.modal {
		return ks
	}
	return append(ks, h.app.Search, h.notifications, h.app.Help, h.app.Quit)
}

func (h helpKeys) FullHelp() [][]key.Binding {
	var groups [][]key.Binding
	if h.section != nil {
		groups = h.section.FullHelp()
	}
	if h.modal {
		return groups
	}
	app := make([]key.Binding, 0, 8)
	if h.panes {
		app = append(app, h.app.Next, h.app.Prev, h.app.jump)
	}
	app = append(app, h.app.Search, h.notifications, h.dismiss, h.app.Help, h.app.Quit)
	return append(groups, app)
}
