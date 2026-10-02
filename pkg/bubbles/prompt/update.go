package prompt

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Update submits or cancels on their keys and passes everything else to the
// input. A blurred prompt ignores every message.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(k, m.keys.Submit),
			m.mode == SingleLine && key.Matches(k, m.keys.SubmitLine):
			return m, m.submit()
		case key.Matches(k, m.keys.Cancel):
			return m, m.cancel()
		}
	}
	var cmd tea.Cmd
	if m.mode == SingleLine {
		// The input edits its text in place, which copies of the model share,
		// so it gets a copy of its own first.
		m.input.SetValue(m.input.Value())
		m.input, cmd = m.input.Update(msg)
	} else {
		m.area, cmd = m.area.Update(msg)
	}
	m.render()
	return m, cmd
}
