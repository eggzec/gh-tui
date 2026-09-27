package keyhelp

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Update handles keys while the help is focused. The query takes every key
// but its own bindings; while a key is captured, every key but Capture
// filters the list instead.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focused {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.press(msg)
	case tea.MouseWheelMsg:
		m.vp, _ = m.vp.Update(msg)
		m.render()
		return m, nil
	}
	// Pastes and the like go to the query.
	return m.typeIn(msg)
}

func (m Model) press(k tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(k, m.keys.Capture):
		m.capturing = !m.capturing
		m.render()
		return m, nil
	case m.capturing:
		m.key = k.String()
		m.refilter()
		return m, nil
	case key.Matches(k, m.keys.Back):
		if !m.filtered() {
			return m, send(CloseMsg{ID: m.id})
		}
		m.input.Reset()
		m.key = ""
		m.refilter()
		return m, nil
	case !m.filtered() && key.Matches(k, m.keys.Close):
		return m, send(CloseMsg{ID: m.id})
	case key.Matches(k, m.keys.Up):
		m.vp.ScrollUp(1)
	case key.Matches(k, m.keys.Down):
		m.vp.ScrollDown(1)
	case key.Matches(k, m.keys.PageUp):
		m.vp.PageUp()
	case key.Matches(k, m.keys.PageDown):
		m.vp.PageDown()
	case key.Matches(k, m.keys.Home):
		m.vp.GotoTop()
	case key.Matches(k, m.keys.End):
		m.vp.GotoBottom()
	default:
		return m.typeIn(k)
	}
	m.render()
	return m, nil
}

// typeIn passes msg to the query, and filters again if it changed.
func (m Model) typeIn(msg tea.Msg) (Model, tea.Cmd) {
	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.refilter()
	} else {
		m.render()
	}
	return m, cmd
}
