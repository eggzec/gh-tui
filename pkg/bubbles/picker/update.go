package picker

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update handles keys while focused, and the picker's own search results,
// debounce ticks and spinner ticks. It ignores messages meant for other
// pickers.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case debounceMsg:
		if msg.id != m.id || msg.seq != m.seq {
			return m, nil
		}
		return m, m.searchCmd()
	case resultMsg:
		if msg.id != m.id || msg.seq != m.seq {
			return m, nil
		}
		m.receive(msg)
		return m, nil
	case spinner.TickMsg:
		if msg.ID != m.spin.ID() {
			return m, nil
		}
		if !m.loading {
			m.spinning = false
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		m.render()
		return m, cmd
	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		cmd := m.press(msg)
		return m, cmd
	case tea.PasteMsg:
		if !m.focused || !m.typingMode() {
			return m, nil
		}
		cmd := m.edit(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) press(msg tea.KeyPressMsg) tea.Cmd {
	normal := m.modes && !m.typing
	switch {
	case key.Matches(msg, m.keys.Choose):
		it, ok := m.Selected()
		if !ok {
			return nil
		}
		m.stop()
		return send(ChosenMsg{ID: m.id, Item: it})
	case key.Matches(msg, m.keys.Cancel):
		if m.modes && m.typing {
			m.leaveTyping()
			return nil
		}
		m.stop()
		return send(CancelMsg{ID: m.id})
	case normal:
		return m.pressNormal(msg)
	case key.Matches(msg, m.keys.Up):
		m.move(-1)
	case key.Matches(msg, m.keys.Down):
		m.move(1)
	case key.Matches(msg, m.keys.PageUp):
		m.move(-max(m.listHeight(), 1))
	case key.Matches(msg, m.keys.PageDown):
		m.move(max(m.listHeight(), 1))
	case key.Matches(msg, m.keys.NextScope):
		m.scope = (m.scope + 1) % (len(m.scopes) + 1)
		return m.refresh(false)
	case key.Matches(msg, m.keys.PrevScope):
		m.scope = (m.scope + len(m.scopes)) % (len(m.scopes) + 1)
		return m.refresh(false)
	default:
		if m.noFilterLine {
			return nil
		}
		return m.edit(msg)
	}
	m.render()
	return nil
}

// pressNormal handles a key in normal mode, where only the moves and the
// keys that focus the input do anything.
func (m *Model) pressNormal(msg tea.KeyPressMsg) tea.Cmd {
	n := m.keys.Normal
	half := max(m.listHeight()/2, 1)
	switch {
	case key.Matches(msg, n.Up):
		m.move(-1)
	case key.Matches(msg, n.Down):
		m.move(1)
	case key.Matches(msg, n.PageUp):
		m.move(-max(m.listHeight(), 1))
	case key.Matches(msg, n.PageDown):
		m.move(max(m.listHeight(), 1))
	case key.Matches(msg, n.HalfPageUp):
		m.move(-half)
	case key.Matches(msg, n.HalfPageDown):
		m.move(half)
	case key.Matches(msg, n.Top):
		m.sel = 0
		m.scroll()
	case key.Matches(msg, n.Bottom):
		m.sel = len(m.results) - 1
		m.scroll()
	case key.Matches(msg, n.Insert):
		return m.enterTyping(false)
	case key.Matches(msg, n.Append):
		return m.enterTyping(true)
	default:
		return nil
	}
	m.render()
	return nil
}

// enterTyping focuses the input, the cursor at the end of the query or at
// its start.
func (m *Model) enterTyping(end bool) tea.Cmd {
	m.typing = true
	cmd := m.input.Focus()
	// The input edits its text in place, which copies of the model share,
	// so it gets a copy of its own first.
	m.input.SetValue(m.input.Value())
	if end {
		m.input.CursorEnd()
	} else {
		m.input.CursorStart()
	}
	m.retype()
	m.render()
	return cmd
}

// leaveTyping blurs the input and returns to normal mode, keeping the
// query.
func (m *Model) leaveTyping() {
	m.typing = false
	m.input.Blur()
	m.retype()
	m.render()
}

// edit passes msg to the input, and looks for the new query if the text
// changed.
func (m *Model) edit(msg tea.Msg) tea.Cmd {
	before := m.input.Value()
	// The input edits its text in place, which copies of the model share,
	// so it gets a copy of its own first.
	m.input.SetValue(before)
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() == before {
		m.render()
		return cmd
	}
	return tea.Batch(cmd, m.refresh(true))
}

func (m *Model) move(delta int) {
	m.sel += delta
	m.scroll()
}

// receive lists what a search returned.
func (m *Model) receive(msg resultMsg) {
	m.loading = false
	m.err = msg.err
	if msg.err != nil {
		m.show(nil)
	} else {
		m.show(searched(msg.items, msg.text))
	}
	m.render()
}

// scroll keeps the selection in range and in view, along with the header
// of its group.
func (m *Model) scroll() {
	m.sel = max(min(m.sel, len(m.results)-1), 0)
	n := m.listHeight()
	if len(m.results) == 0 || n <= 0 {
		m.top = 0
		return
	}
	r := m.itemRow[m.sel]
	first := r
	if r > 0 && m.rows[r-1].item < 0 && n > 1 {
		first = r - 1
	}
	m.top = min(m.top, first)
	if r >= m.top+n {
		m.top = r - n + 1
	}
	m.top = max(min(m.top, len(m.rows)-n), 0)
}
