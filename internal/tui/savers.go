package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// saveOpen saves what the open modal and the focused section hold unsent,
// if they are a [ui.Saver]. It runs as the app quits, in Update, since the
// text it saves is the model's and the files are small.
func (m *Model) saveOpen() {
	var savers []ui.Saver
	if s, ok := m.topModal().(ui.Saver); ok {
		savers = append(savers, s)
	}
	if p := m.focused(); p != nil {
		if s, ok := p.section.(ui.Saver); ok {
			savers = append(savers, s)
		}
	}
	for _, s := range savers {
		if err := s.Save(); err != nil {
			slog.WarnContext(m.ctx, "unsent text not saved", "span", "tui", "err", err.Error())
		}
	}
}

// quitNow saves what is open and quits at once.
func (m *Model) quitNow() tea.Cmd {
	m.saveOpen()
	return tea.Quit
}
