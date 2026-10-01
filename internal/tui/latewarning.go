package tui

import tea "charm.land/bubbletea/v2"

// WithLateWarning sets where the app hears of a warning that work begun
// at startup finds once the app runs, such as whose a token from the
// environment is, which it tells the user of once.
func WithLateWarning(late <-chan string) Option {
	return func(m *Model) { m.lateWarning = late }
}

// lateWarningMsg is a warning found once the app runs.
type lateWarningMsg struct {
	text string
}

// listenLateWarning waits to hear the warning of WithLateWarning. It
// hears once: the work that tells it runs once a session.
func (m *Model) listenLateWarning() tea.Cmd {
	if m.lateWarning == nil {
		return nil
	}
	ch, ctx := m.lateWarning, m.ctx
	return func() tea.Msg {
		select {
		case text := <-ch:
			return lateWarningMsg{text: text}
		case <-ctx.Done():
			return nil
		}
	}
}
