package pager

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update scrolls and searches on keys while the pager is focused, and takes
// the highlighted tokens of its content and spins while loading whether
// or not it is. Rendered content renders again if what it did changed the
// width of the text, such as an inverted search, whose marks widen the
// gutter.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	m, cmd := m.update(msg)
	m.fitRendered(false)
	return m, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case settledMsg:
		if msg.id == m.id && msg.seq == m.sizeSeq {
			m.owed = false
		}
		return m, nil
	case highlightMsg:
		if msg.id == m.id && msg.gen == m.gen {
			m.spans = msg.spans
		}
		return m, nil
	case searchMsg:
		if msg.id == m.id && msg.qgen == m.qgen && m.search.running {
			m.found(msg.lines, msg.ends)
		}
		return m, nil
	case projectMsg:
		if msg.id == m.id && msg.pgen == m.pgen && m.projecting {
			cmd := m.picked(msg.vis, msg.kept)
			return m, cmd
		}
		return m, nil
	case editedMsg:
		if msg.id == m.id {
			m.edited(msg.err)
		}
		return m, nil
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
		m.flash, m.flashInfo = "", false
		switch {
		case m.prompt.Focused():
			return m.updatePrompt(msg)
		case m.opt:
			return m.updateOption(msg)
		}
		return m.updateKey(msg)
	}
	if m.prompt.Focused() {
		// Pastes and the like go to the prompt.
		return m.updatePrompt(msg)
	}
	return m, nil
}

// maxCount is the largest count, so typing digits can't overflow it.
const maxCount = 1<<31 - 1

func (m Model) updateKey(k tea.KeyPressMsg) (Model, tea.Cmd) {
	// A count lasts one key: g and G go to its line, esc drops it, and
	// any other key acts as it would without it.
	n, counted := m.num, m.counting
	if counted {
		cancel := key.Matches(k, m.keys.Cancel)
		m.num, m.counting = 0, false
		m.enableSearchKeys()
		if cancel {
			return m, nil
		}
	}
	h := max(m.bodyHeight(), 1)
	switch {
	case key.Matches(k, m.keys.Count):
		if d := int(k.Code - '0'); d >= 0 && d <= 9 {
			m.num, m.counting = min(n*10+d, maxCount), true
			m.enableSearchKeys()
		}
	case counted && key.Matches(k, m.keys.Home, m.keys.End):
		m.goTo(n)
	case counted && key.Matches(k, m.keys.Percent):
		m.goToPercent(n)
	case key.Matches(k, m.keys.Down):
		m.down(1)
	case key.Matches(k, m.keys.Up):
		m.up(1)
	case key.Matches(k, m.keys.PageDown):
		m.down(h)
	case key.Matches(k, m.keys.PageUp):
		m.up(h)
	case key.Matches(k, m.keys.HalfPageDown):
		m.down(max(h/2, 1))
	case key.Matches(k, m.keys.HalfPageUp):
		m.up(max(h/2, 1))
	case key.Matches(k, m.keys.Home):
		m.top, m.row = 0, 0
		m.clamp()
	case key.Matches(k, m.keys.End):
		m.top, m.row = m.last()
		m.clamp()
	case key.Matches(k, m.keys.Right):
		if !m.wrap {
			m.scrollRight(m.hStep())
			m.findHits()
		}
	case key.Matches(k, m.keys.Left):
		m.left = max(m.left-m.hStep(), 0)
		m.findHits()
	case key.Matches(k, m.keys.Option):
		m.opt = true
		m.enableSearchKeys()
	case key.Matches(k, m.keys.Search):
		cmd := m.openPrompt(promptSearch)
		return m, cmd
	case key.Matches(k, m.keys.Filter):
		cmd := m.openPrompt(promptFilter)
		return m, cmd
	case key.Matches(k, m.keys.Next):
		m.step(1)
	case key.Matches(k, m.keys.Prev):
		m.step(-1)
	case key.Matches(k, m.keys.Cancel):
		// Esc peels one layer at a time: the search, then the filter.
		if m.search.query != "" {
			m.clearSearch()
			m.clamp()
			return m, nil
		}
		cmd := m.clearFilter()
		return m, cmd
	case key.Matches(k, m.keys.Edit):
		// A count before it is dropped, as it means nothing to the editor.
		if !counted {
			cmd := m.edit()
			return m, cmd
		}
	case key.Matches(k, m.keys.Close):
		return m, m.close()
	}
	return m, nil
}
