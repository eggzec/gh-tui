package keyhelp

import tea "charm.land/bubbletea/v2"

// CloseMsg reports that the user closed the help with ID.
type CloseMsg struct {
	ID int64
}

func send(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
