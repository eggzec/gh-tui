package pager

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
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

// own handles the messages the pager sends itself, and reports whether msg
// was one of them.
func (m Model) own(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case settledMsg:
		if msg.id == m.id && msg.seq == m.sizeSeq {
			m.owed = false
		}
	case highlightMsg:
		if msg.id == m.id && msg.gen == m.gen {
			m.spans = msg.spans
		}
	case searchMsg:
		if msg.id == m.id && msg.qgen == m.qgen && m.search.running {
			m.found(msg.lines, msg.ends)
		}
	case projectMsg:
		if msg.id == m.id && msg.pgen == m.pgen && m.projecting {
			cmd := m.picked(msg.vis, msg.kept)
			return m, cmd, true
		}
	case editedMsg:
		if msg.id == m.id {
			m.edited(msg.err)
		}
	case spinner.TickMsg:
		if m.state != stateLoading {
			return m, nil, true
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd, true
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	if out, cmd, ok := m.own(msg); ok {
		return out, cmd
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		if !m.focused {
			return m, nil
		}
		m.flash, m.flashInfo = "", false
		switch {
		case m.prompt.Focused():
			return m.updatePrompt(k)
		case m.opt:
			return m.updateOption(k)
		case m.hostTakes():
			return m.handle(k)
		}
		return m.updateKey(k)
	}
	if m.prompt.Focused() {
		// Pastes and the like go to the prompt.
		return m.updatePrompt(msg)
	}
	return m, nil
}

func (m Model) updateKey(k tea.KeyPressMsg) (Model, tea.Cmd) {
	h := max(m.bodyHeight(), 1)
	switch {
	case keymap.Matches(k, m.keys.Down):
		m.down(1)
	case keymap.Matches(k, m.keys.Up):
		m.up(1)
	case keymap.Matches(k, m.keys.PageDown):
		m.down(h)
	case keymap.Matches(k, m.keys.PageUp):
		m.up(h)
	case keymap.Matches(k, m.keys.HalfPageDown):
		m.down(max(h/2, 1))
	case keymap.Matches(k, m.keys.HalfPageUp):
		m.up(max(h/2, 1))
	case keymap.Matches(k, m.keys.Home):
		m.top, m.row = 0, 0
		m.clamp()
	case keymap.Matches(k, m.keys.End):
		m.top, m.row = m.last()
		m.clamp()
	case keymap.Matches(k, m.keys.Right):
		if !m.wrap {
			m.scrollRight(m.hStep())
			m.findHits()
		}
	case keymap.Matches(k, m.keys.Left):
		m.left = max(m.left-m.hStep(), 0)
		m.findHits()
	case keymap.Matches(k, m.keys.Option):
		m.opt = true
		m.enableSearchKeys()
	case keymap.Matches(k, m.keys.Search):
		cmd := m.openPrompt(promptSearch)
		return m, cmd
	case keymap.Matches(k, m.keys.Filter):
		cmd := m.openPrompt(promptFilter)
		return m, cmd
	case keymap.Matches(k, m.keys.Next):
		m.step(1)
	case keymap.Matches(k, m.keys.Prev):
		m.step(-1)
	case keymap.Matches(k, m.keys.Edit):
		cmd := m.edit()
		return m, cmd
	case keymap.Matches(k, m.keys.Quit, m.keys.Dismiss):
		return m, m.close()
	default:
		return m.handle(k)
	}
	return m, nil
}

// handle gives a key to the host's handler, if there is one, and keeps the
// pager as it was when the handler leaves the key.
func (m Model) handle(k tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.handler == nil {
		return m, nil
	}
	if out, cmd, ok := m.handler(m, k); ok {
		return out, cmd
	}
	return m, nil
}
