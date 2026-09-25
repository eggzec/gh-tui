package finder

import (
	"context"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update handles keys while focused, and the finder's own load, matches
// and spinner ticks. It ignores messages meant for other finders.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		if msg.id != m.id || !m.loading {
			return m, nil
		}
		m.loading, m.err, m.note = false, msg.err, msg.note
		if msg.err != nil {
			m.render()
			return m, nil
		}
		m.corpus = msg.corpus
		m.recent = m.corpus.ranks(m.recentPaths)
		cmd := m.match()
		return m, cmd
	case matchMsg:
		if msg.id != m.id || msg.seq != m.seq {
			return m, nil
		}
		m.show(msg.res)
		return m, nil
	case spinner.TickMsg:
		if msg.ID != m.spin.ID() {
			return m, nil
		}
		if !m.loading && !m.matching {
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
		if !m.focused {
			return m, nil
		}
		cmd := m.edit(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) press(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Choose):
		it, ok := m.Selected()
		if !ok {
			return nil
		}
		return send(ChosenMsg{ID: m.id, Item: it})
	case key.Matches(msg, m.keys.Cancel):
		return send(CancelMsg{ID: m.id})
	case key.Matches(msg, m.keys.Up):
		m.move(-1)
	case key.Matches(msg, m.keys.Down):
		m.move(1)
	case key.Matches(msg, m.keys.PageUp):
		m.move(-max(m.listHeight(), 1))
	case key.Matches(msg, m.keys.PageDown):
		m.move(max(m.listHeight(), 1))
	default:
		return m.edit(msg)
	}
	m.render()
	return nil
}

// edit passes msg to the input, and matches the query if it changed.
func (m *Model) edit(msg tea.Msg) tea.Cmd {
	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() == before {
		m.render()
		return cmd
	}
	return tea.Batch(cmd, m.match())
}

// match matches the query, from the matches of the last result if the
// query narrows it. A few candidates are matched at once; more are matched
// in a command, while the last result stays on screen.
func (m *Model) match() tea.Cmd {
	m.seq++
	m.stop()
	m.stop, m.matching = func() {}, false
	q := m.input.Value()
	if m.corpus == nil || m.res != nil && m.res.query == q {
		m.render()
		return nil
	}
	from := m.from(q)
	n := m.corpus.len()
	if from != nil && !from.all {
		n = len(from.items)
	}
	c, recent := m.corpus, m.recent
	if n <= m.syncLimit || len(terms(q)) == 0 {
		res, _ := filter(context.Background(), c, q, from, recent)
		m.show(res)
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.stop, m.matching = cancel, true
	id, seq := m.id, m.seq
	m.render()
	return tea.Batch(func() tea.Msg {
		defer cancel()
		res, ok := filter(ctx, c, q, from, recent)
		if !ok {
			return nil
		}
		return matchMsg{id: id, seq: seq, res: res}
	}, m.tick())
}

// from returns the result that q need only look through, or nil for all
// the items.
func (m *Model) from(q string) *result {
	if m.res == nil || !narrows(m.res.terms, terms(q)) {
		return nil
	}
	return m.res
}

// show lists res with its best match selected.
func (m *Model) show(res *result) {
	m.res, m.matching = res, false
	m.sel, m.top = 0, 0
	m.rows = nil
	m.render()
}

// tick starts the spinner unless it is running.
func (m *Model) tick() tea.Cmd {
	if m.spinning {
		return nil
	}
	m.spinning = true
	return m.spin.Tick
}

func (m *Model) move(delta int) {
	m.sel += delta
	m.scroll()
}

// scroll keeps the selection in range and in view.
func (m *Model) scroll() {
	n := m.Matches()
	m.sel = max(min(m.sel, n-1), 0)
	h := m.listHeight()
	if h <= 0 {
		m.top = 0
		return
	}
	m.top = min(m.top, m.sel)
	if m.sel >= m.top+h {
		m.top = m.sel - h + 1
	}
	m.top = max(min(m.top, n-h), 0)
}
