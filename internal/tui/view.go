package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
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
	return m.help.View(m.hints())
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
	return lipgloss.Height(m.help.View(m.hints()))
}

// hints is the key map of the help line: the keys that reach something
// now, with the app's own after those of what has the focus.
func (m *Model) hints() ui.Hints {
	// The way out of a zoom leads, where a narrow line still shows it.
	return ui.Hints{Layers: m.keyLayers(), Lead: []key.Binding{m.keys.state(m).Back}}
}

// keyLayers returns the keys the app and what has the focus take, in the
// order a key reaches them: an open modal takes every key, and so does a
// section while it captures them, but for the one that quits; otherwise
// the keys the focused section claims come first, then the app's, and
// then the section's.
func (m *Model) keyLayers() []keyhelp.Layer {
	always := keyhelp.Layer{Source: "app", Bindings: []key.Binding{forceQuit}}
	if mod := m.topModal(); mod != nil {
		return append([]keyhelp.Layer{always}, mod.KeyLayers()...)
	}
	app := keyhelp.FromHelp("app", m.keys.state(m), false)
	p := m.focused()
	if p == nil {
		return []keyhelp.Layer{app}
	}
	if c, ok := p.section.(ui.Capturer); ok && c.Capturing() {
		return append([]keyhelp.Layer{always}, p.section.KeyLayers()...)
	}
	var layers []keyhelp.Layer
	if c, ok := p.section.(ui.Claimer); ok {
		layers = append(layers, keyhelp.Layer{Source: p.section.Title(), Bindings: c.Claimed()})
	}
	layers = append(layers, app)
	return append(layers, p.section.KeyLayers()...)
}
