// Package tui is the glue between services and bubbles. It owns layout and
// routes messages to views.
package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var titleStyle = lipgloss.NewStyle().Bold(true).Padding(0, 1)

// Model is the root model of the program.
type Model struct {
	keys          KeyMap
	help          help.Model
	width, height int
}

// New returns a root model with default key bindings.
func New() *Model {
	return &Model{
		keys: DefaultKeyMap(),
		help: help.New(),
	}
}

// Init asks for the terminal background so styles can match it.
func (m *Model) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

// Update handles global messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
	case tea.BackgroundColorMsg:
		m.help.Styles = help.DefaultStyles(msg.IsDark())
	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the full screen.
func (m *Model) View() tea.View {
	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("gh-tui"),
		m.help.View(m.keys),
	))
	v.AltScreen = true
	v.WindowTitle = "gh-tui"
	return v
}
