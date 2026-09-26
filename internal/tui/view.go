package tui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/overlay"
)

// View lays out the header, the screen on view and the help line, or the
// command line in its place, with the top modal and the toasts over them.
func (m *Model) View() tea.View {
	// The footer is rendered once, and its height is that of what it
	// shows.
	footer := m.footer()
	body := m.body()
	pad := max(m.height-1-lipgloss.Height(footer)-len(body), 0)
	n := len(m.header) + len(body) + pad + 1 + len(footer)
	for _, l := range body {
		n += len(l)
	}
	var b strings.Builder
	b.Grow(n)
	b.WriteString(m.header)
	for _, l := range body {
		b.WriteByte('\n')
		b.WriteString(l)
	}
	// The screen fills its height even when it has nothing to show, so the
	// help line stays at the bottom.
	for range pad {
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(footer)

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
	m.line.SetSize(m.width, cmdline.MaxHeight)
	m.toast.SetSize(m.width, m.height)
	m.arrange(m.contentHeight())
	if m.modal != nil {
		m.modal.SetSize(m.modalSize())
	}
	m.drawFrames()
	m.drawHeader()
}

// contentHeight is the height between the header and the help line.
func (m *Model) contentHeight() int {
	return max(m.height-1-m.footerHeight(), 0)
}

// footer is what the bottom of the screen shows: the command line while it
// is open, a spinner while a goto waits for GitHub, and the help line
// otherwise.
func (m *Model) footer() string {
	switch {
	case m.line.Focused():
		return m.line.View()
	case m.going != nil:
		return m.goingView()
	}
	return m.help.View(m.helpKeys())
}

// footerHeight is the height of the footer.
func (m *Model) footerHeight() int {
	switch {
	case m.line.Focused():
		return m.line.Height()
	case m.going != nil:
		return 1
	}
	return m.helpHeight()
}

func (m *Model) helpHeight() int {
	return lipgloss.Height(m.help.View(m.helpKeys()))
}

func (m *Model) helpKeys() help.KeyMap {
	hk := helpKeys{
		app: m.keys, dismiss: m.toast.KeyMap().Dismiss, notifications: m.keys.Notifications,
		history: m.keys.History, actions: m.keys.Actions, dashboard: m.keys.Dashboard,
		findFile: m.keys.FindFile, zoom: m.keys.Zoom, unzoom: m.keys.Back,
	}
	if m.fileFinder() == nil {
		hk.findFile.SetEnabled(false)
	}
	if !m.canOpenHistory() {
		hk.history.SetEnabled(false)
	}
	if !m.canOpenActions() {
		hk.actions.SetEnabled(false)
	}
	switch m.screen {
	case notifScreen:
		hk.notifications.SetHelp(hk.notifications.Help().Key, "back")
	case dashScreen:
		hk.dashboard.SetHelp(hk.dashboard.Help().Key, "back")
		if m.back == dashScreen {
			hk.dashboard.SetEnabled(false)
		}
	case repoScreen, searchScreen:
	}
	if m.dash == nil {
		hk.dashboard.SetEnabled(false)
	}
	if mod := m.topModal(); mod != nil {
		// The app's own keys don't reach a modal.
		hk.section, hk.modal = mod.Help(), true
		return hk
	}
	if p := m.focused(); p != nil {
		hk.section = p.section.Help()
		if c, ok := p.section.(ui.Capturer); ok && c.Capturing() {
			// The command key types itself there.
			hk.app.Command.SetEnabled(false)
		}
	}
	hk.panes = m.canZoom()
	if m.width < narrowWidth {
		hk.zoom.SetEnabled(false)
	}
	if !hk.panes || !m.zoomed() {
		hk.unzoom.SetEnabled(false)
	}
	return hk
}

// helpKeys lists the keys of the focused pane before the app's own.
type helpKeys struct {
	section       help.KeyMap
	app           KeyMap
	dismiss       key.Binding
	notifications key.Binding
	history       key.Binding
	actions       key.Binding
	findFile      key.Binding
	dashboard     key.Binding
	// zoom shows the focused pane alone, where that shows, and unzoom
	// every pane again while one is zoomed.
	zoom, unzoom key.Binding
	// modal hides the app's keys while a modal takes them.
	modal bool
	// panes shows the keys that move between panes and zoom them.
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
	// The way out of a zoom comes first, where a narrow help still shows
	// it.
	ks = append([]key.Binding{h.unzoom}, h.untaken(ks)...)
	return append(ks, h.app.Search, h.app.Command, h.findFile, h.history, h.actions, h.notifications, h.dashboard, h.app.Help, h.app.Quit)
}

func (h helpKeys) FullHelp() [][]key.Binding {
	var groups [][]key.Binding
	if h.section != nil {
		groups = h.section.FullHelp()
	}
	if h.modal {
		return groups
	}
	groups = slices.Clone(groups)
	for i, g := range groups {
		groups[i] = h.untaken(g)
	}
	app := make([]key.Binding, 0, 16)
	if h.panes {
		app = append(app, h.app.Next, h.app.Prev, h.app.jump, h.zoom, h.unzoom)
	}
	app = append(app, h.app.Search, h.app.Command, h.findFile, h.history, h.actions, h.notifications, h.dashboard, h.dismiss, h.app.Help, h.app.Quit)
	return append(groups, app)
}

// untaken drops the bindings of the section that share a key with those
// the app takes before it, such as its back key while the app unzooms.
func (h helpKeys) untaken(bs []key.Binding) []key.Binding {
	if !h.unzoom.Enabled() {
		return bs
	}
	taken := h.unzoom.Keys()
	return slices.DeleteFunc(slices.Clone(bs), func(b key.Binding) bool {
		return b.Enabled() && slices.ContainsFunc(b.Keys(), func(k string) bool { return slices.Contains(taken, k) })
	})
}
