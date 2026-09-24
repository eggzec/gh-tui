package main

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// placeholder holds the place of a section that is yet to come, with a line
// that says so.
type placeholder struct {
	title, text   string
	width, height int
	muted         lipgloss.Style
	view          string
}

func (p *placeholder) Title() string          { return p.title }
func (p *placeholder) Init() tea.Cmd          { return nil }
func (p *placeholder) Update(tea.Msg) tea.Cmd { return nil }
func (p *placeholder) View() string           { return p.view }
func (p *placeholder) Focus()                 {}
func (p *placeholder) Blur()                  {}
func (p *placeholder) Help() help.KeyMap      { return noKeys{} }

func (p *placeholder) SetTheme(t ui.Theme) {
	p.muted = t.Muted
	p.render()
}

func (p *placeholder) SetSize(width, height int) {
	p.width, p.height = width, height
	p.render()
}

func (p *placeholder) render() {
	if p.width <= 0 || p.height <= 0 {
		p.view = ""
		return
	}
	line := lipgloss.PlaceHorizontal(p.width, lipgloss.Center, p.muted.MaxWidth(p.width).Render(p.text))
	p.view = lipgloss.PlaceVertical(p.height, lipgloss.Center, line)
}

type noKeys struct{}

func (noKeys) ShortHelp() []key.Binding  { return nil }
func (noKeys) FullHelp() [][]key.Binding { return nil }
