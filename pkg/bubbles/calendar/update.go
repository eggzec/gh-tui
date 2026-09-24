package calendar

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Update moves the cursor on keys while focused, and sends a [SelectMsg]
// when it lands on another day. The calendar has no messages of its own, so
// it ignores everything else.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.focused || len(m.grid) == 0 {
		return m, nil
	}
	i := m.cw*7 + m.cd
	switch {
	case key.Matches(k, m.keyMap.Up):
		i, _ = m.step(i, -1)
	case key.Matches(k, m.keyMap.Down):
		i, _ = m.step(i, 1)
	case key.Matches(k, m.keyMap.Left):
		i = m.sideways(-1)
	case key.Matches(k, m.keyMap.Right):
		i = m.sideways(1)
	case key.Matches(k, m.keyMap.First):
		i, _ = m.step(-1, 1)
	case key.Matches(k, m.keyMap.Last):
		i, _ = m.step(len(m.grid)*7, -1)
	default:
		return m, nil
	}
	if i == m.cw*7+m.cd {
		return m, nil
	}
	m.move(i/7, i%7)
	id, day := m.id, m.grid[m.cw][m.cd].day
	return m, func() tea.Msg { return SelectMsg{ID: id, Day: day} }
}

// step returns the index, week*7+weekday, of the first day from i in the
// direction dir, not counting i. It returns i and false if there is none.
func (m Model) step(i, dir int) (int, bool) {
	for j := i + dir; j >= 0 && j < len(m.grid)*7; j += dir {
		if m.grid[j/7][j%7].ok {
			return j, true
		}
	}
	return i, false
}

// sideways returns the index of the day in the week before or after the
// cursor's, on the same weekday if it has one and on the nearest otherwise,
// as at the ends of partial weeks.
func (m Model) sideways(dir int) int {
	w := m.cw + dir
	if w < 0 || w >= len(m.grid) {
		return m.cw*7 + m.cd
	}
	for dist := range 7 {
		for _, d := range [2]int{m.cd - dist, m.cd + dist} {
			if d >= 0 && d < 7 && m.grid[w][d].ok {
				return w*7 + d
			}
		}
	}
	return m.cw*7 + m.cd
}

// move puts the cursor on week w and weekday d, and renders only the lines
// that change unless the weeks shown scroll.
func (m *Model) move(w, d int) {
	old := m.cd
	m.cw, m.cd = w, d
	if m.follow() {
		m.render()
		return
	}
	// Copies of the model share the lines, so change a copy.
	m.lines = slices.Clone(m.lines)
	m.renderRow(old)
	m.renderRow(d)
	m.renderFooter()
}
