package toast

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Update expires toasts and handles the dismiss key. It ignores messages
// meant for other instances.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ExpireMsg:
		if msg.id == m.id {
			m.expire(msg.seq)
		}
	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Dismiss) {
			cmd := m.Dismiss()
			return m, cmd
		}
	}
	return m, nil
}
