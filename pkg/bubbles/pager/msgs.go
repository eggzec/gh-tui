package pager

import tea "charm.land/bubbletea/v2"

// CloseMsg asks the parent to close the pager with ID, for example the
// modal it is shown in.
type CloseMsg struct {
	ID int64
}

func (m Model) close() tea.Cmd {
	msg := CloseMsg{ID: m.id}
	return func() tea.Msg { return msg }
}
