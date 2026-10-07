package toast

import tea "charm.land/bubbletea/v2"

// Update expires toasts. It ignores messages meant for other instances. The
// parent dismisses the newest toast with [Model.Dismiss], on a key of its
// own.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if msg, ok := msg.(ExpireMsg); ok && msg.id == m.id {
		m.expire(msg.seq)
	}
	return m, nil
}
