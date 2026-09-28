package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/overlay"
)

// View lays out the header, the screen on view and the status bar, or the
// command line in its place, with the top modal over them, the help over
// that, and the toasts over all but the last line, which the status bar
// keeps.
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
	// footer stays at the bottom.
	for range pad {
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(footer)

	screen := b.String()
	mod := m.topModal()
	if mod != nil || m.helpOpen() {
		// The modal and the help are centered on the terminal, so the
		// screen under them must fill it.
		screen = lipgloss.PlaceVertical(m.height, lipgloss.Top, screen)
	}
	if mod != nil {
		screen = overlay.Center(screen, m.frame(mod), m.width, m.height)
	}
	if m.helpOpen() {
		screen = overlay.Center(screen, m.helpFrame(), m.width, m.height)
	}
	if !m.toast.Empty() {
		screen = m.overToasts(screen)
	}
	v := tea.NewView(screen)
	v.AltScreen = true
	v.ReportFocus = true
	v.WindowTitle = "gh-tui"
	return v
}

// overToasts draws the toasts over screen, but for its last line, which
// the status bar keeps.
func (m *Model) overToasts(screen string) string {
	if i := strings.LastIndexByte(screen, '\n'); i >= 0 && m.height > 1 {
		return m.toast.Overlay(screen[:i], m.width, m.height-1) + screen[i:]
	}
	return m.toast.Overlay(screen, m.width, m.height)
}

// layout gives each part its share of the screen.
func (m *Model) layout() {
	m.status.SetWidth(m.width)
	m.line.SetSize(m.width, cmdline.MaxHeight)
	m.toast.SetSize(m.width, max(m.height-1, 0))
	m.arrange(m.contentHeight())
	if m.modal != nil {
		m.modal.SetSize(m.modalSize())
	}
	m.keyhelp.SetSize(m.helpSize())
	m.drawFrames()
	m.drawHeader()
}

// contentHeight is the height between the header and the footer.
func (m *Model) contentHeight() int {
	return max(m.height-1-m.footerHeight(), 0)
}

// footer is what the bottom of the screen shows: the command line while it
// is open, a spinner while a goto waits for GitHub, and the status bar
// otherwise.
func (m *Model) footer() string {
	switch {
	case m.line.Focused():
		return m.line.View()
	case m.going != nil:
		return m.goingView()
	}
	return m.bar()
}

// footerHeight is the height of the footer.
func (m *Model) footerHeight() int {
	if m.line.Focused() {
		return m.line.Height()
	}
	// The spinner of a goto and the status bar are a line each.
	return 1
}
