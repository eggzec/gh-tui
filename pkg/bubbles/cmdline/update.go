package cmdline

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Update handles the command line's keys and passes everything else to the
// input. A change to the line or a move of the cursor asks Complete for
// new candidates, and a change, or a candidate inserted, ends a walk
// through the history. A blurred command line ignores every message.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	switch msg := msg.(type) {
	case SubmitMsg, CancelMsg:
		// Results, of this command line or another, are the parent's.
		return m, nil
	case tea.FocusMsg, tea.BlurMsg:
		// The parent owns focus, through Focus and Blur; the terminal's
		// focus mustn't hide the cursor behind its back.
		return m, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Submit):
			cmd := m.submit()
			return m, cmd
		case key.Matches(msg, m.keys.Cancel),
			m.input.Value() == "" && key.Matches(msg, m.keys.CancelEmpty):
			cmd := m.cancel()
			return m, cmd
		case key.Matches(msg, m.keys.Next):
			m.walk = walk{}
			m.cycle(1)
			m.render()
			return m, nil
		case key.Matches(msg, m.keys.Prev):
			m.walk = walk{}
			m.cycle(-1)
			m.render()
			return m, nil
		case key.Matches(msg, m.keys.Older):
			m.older()
			m.render()
			return m, nil
		case key.Matches(msg, m.keys.Newer):
			m.newer()
			m.render()
			return m, nil
		}
	}
	value, pos := m.input.Value(), m.input.Position()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != value {
		// Editing ends a walk through the history.
		m.walk = walk{}
	}
	// The cursor doesn't blink, so only a change to the line or a move of
	// the cursor changes the view.
	if m.input.Value() != value || m.input.Position() != pos {
		m.refresh()
		m.render()
	}
	return m, cmd
}
