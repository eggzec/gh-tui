package logview

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update scrolls, folds and searches on keys while the view is focused,
// and spins while loading whether or not it is.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.state != stateLoading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		if m.searching {
			return m.updateSearch(msg)
		}
		return m.updateKey(msg)
	}
	if m.focused && m.searching {
		// Pastes and the like go to the input.
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateKey(k tea.KeyPressMsg) (Model, tea.Cmd) {
	h := max(m.bodyHeight(), 1)
	switch {
	case key.Matches(k, m.keys.Down):
		m.move(1)
	case key.Matches(k, m.keys.Up):
		m.move(-1)
	case key.Matches(k, m.keys.PageDown):
		m.page(h)
	case key.Matches(k, m.keys.PageUp):
		m.page(-h)
	case key.Matches(k, m.keys.HalfPageDown):
		m.page(max(h/2, 1))
	case key.Matches(k, m.keys.HalfPageUp):
		m.page(-max(h/2, 1))
	case key.Matches(k, m.keys.Home):
		m.home()
	case key.Matches(k, m.keys.End):
		m.end()
	case key.Matches(k, m.keys.Right):
		if !m.wrap {
			m.scrollRight(m.hStep())
		}
	case key.Matches(k, m.keys.Left):
		m.left = max(m.left-m.hStep(), 0)
	case key.Matches(k, m.keys.Toggle):
		m.toggle()
	case key.Matches(k, m.keys.Expand):
		m.expand()
	case key.Matches(k, m.keys.Collapse):
		m.collapse()
	case key.Matches(k, m.keys.ExpandAll):
		m.ExpandAll()
	case key.Matches(k, m.keys.CollapseAll):
		m.CollapseAll()
	case key.Matches(k, m.keys.NextError):
		m.stepIssue(Error, 1)
	case key.Matches(k, m.keys.PrevError):
		m.stepIssue(Error, -1)
	case key.Matches(k, m.keys.NextWarning):
		m.stepIssue(Warning, 1)
	case key.Matches(k, m.keys.PrevWarning):
		m.stepIssue(Warning, -1)
	case key.Matches(k, m.keys.Wrap):
		m.SetWrap(!m.wrap)
	case key.Matches(k, m.keys.Times):
		m.SetTimeMode(m.times.next())
	case key.Matches(k, m.keys.LineNumbers):
		m.SetLineNumbers(!m.lineNumbers)
	case key.Matches(k, m.keys.Follow):
		m.SetFollow(!m.follow)
	case key.Matches(k, m.keys.Search):
		cmd := m.openSearch()
		return m, cmd
	case key.Matches(k, m.keys.Next):
		m.step(1)
	case key.Matches(k, m.keys.Prev):
		m.step(-1)
	case m.search.query != "" && key.Matches(k, m.keys.Cancel):
		m.clearSearch()
	case key.Matches(k, m.keys.Close):
		return m, m.close()
	}
	return m, nil
}

func (m Model) updateSearch(k tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(k, m.keys.Confirm):
		query := m.input.Value()
		m.closeSearch()
		m.runSearch(query)
		return m, nil
	case key.Matches(k, m.keys.Cancel):
		m.closeSearch()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}
