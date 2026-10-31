package orderpick

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// Update handles keys while the picker is focused, and passes the other
// messages, such as the left pane's own, on to it. It ignores messages meant
// for other pickers.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.left, cmd = m.left.Update(msg)
		return m, cmd
	}
	if !m.focused {
		return m, nil
	}
	if m.pane == PaneAvailable {
		return m.pressLeft(press)
	}
	return m.pressRight(press)
}

// pressLeft handles a key of the left pane. The panes take enter and esc
// themselves, so the left picker never sends its own messages, except while
// its search types: there they keep the query and clear it, as in a picker.
func (m Model) pressLeft(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := m.keys
	if !m.left.Typing() {
		switch {
		case keymap.Matches(msg, k.NextPane, k.PrevPane):
			cmd := m.setPane(PaneChosen)
			return m, cmd
		case keymap.Matches(msg, k.Picker.Choose):
			return m, m.confirm()
		case keymap.Matches(msg, k.Picker.Cancel) && m.left.Query().Text == "":
			return m, m.Cancel()
		case keymap.Matches(msg, k.Picker.Normal.Back):
			return m, m.Cancel()
		case keymap.Matches(msg, k.Picker.Normal.Toggle):
			m.tick(msg)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.left, cmd = m.left.Update(msg)
	return m, cmd
}

// pressRight handles a key of the right pane.
func (m Model) pressRight(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k, n := m.keys, m.keys.Picker.Normal
	page := max(m.rows(), 1)
	switch {
	case keymap.Matches(msg, k.NextPane, k.PrevPane):
		cmd := m.setPane(PaneAvailable)
		return m, cmd
	case keymap.Matches(msg, k.Picker.Choose):
		return m, m.confirm()
	case keymap.Matches(msg, k.Picker.Cancel, n.Back):
		return m, m.Cancel()
	case keymap.Matches(msg, k.MoveDown):
		m.shift(1)
	case keymap.Matches(msg, k.MoveUp):
		m.shift(-1)
	case keymap.Matches(msg, k.Flip):
		m.flip()
	case keymap.Matches(msg, k.Remove):
		m.remove()
	case keymap.Matches(msg, n.Up):
		m.moveCursor(m.cursor - 1)
	case keymap.Matches(msg, n.Down):
		m.moveCursor(m.cursor + 1)
	case keymap.Matches(msg, n.PageUp):
		m.moveCursor(m.cursor - page)
	case keymap.Matches(msg, n.PageDown):
		m.moveCursor(m.cursor + page)
	case keymap.Matches(msg, n.HalfPageUp):
		m.moveCursor(m.cursor - max(page/2, 1))
	case keymap.Matches(msg, n.HalfPageDown):
		m.moveCursor(m.cursor + max(page/2, 1))
	case keymap.Matches(msg, n.Top):
		m.moveCursor(0)
	case keymap.Matches(msg, n.Bottom):
		m.moveCursor(len(m.chosen) - 1)
	}
	return m, nil
}

// confirm sends the chosen items, in order.
func (m Model) confirm() tea.Cmd {
	return send(ChosenMsg{ID: m.id, Choices: m.Chosen()})
}
