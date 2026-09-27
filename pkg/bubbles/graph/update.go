package graph

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update handles keys while focused, and the graph's own fetch results and
// spinner ticks. It ignores messages meant for other graphs. It sends a
// [SelectMsg] whenever the cursor lands on another commit.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.resized {
		// SetSize cannot return a command, so fetch what a larger window
		// shows now.
		m.resized = false
		cmd = m.sync()
	}
	switch msg := msg.(type) {
	case chunkMsg:
		if msg.id == m.id && msg.gen == m.gen {
			cmd = tea.Batch(cmd, m.receive(msg))
		}
	case spinner.TickMsg:
		if msg.ID != m.spin.ID() {
			break
		}
		if !m.fetching {
			m.spinning = false
			break
		}
		var tick tea.Cmd
		m.spin, tick = m.spin.Update(msg)
		cmd = tea.Batch(cmd, tick)
	case tea.KeyPressMsg:
		if m.focused {
			cmd = tea.Batch(cmd, m.press(msg))
		}
	}
	return m, tea.Batch(cmd, m.announce())
}

func (m *Model) press(msg tea.KeyPressMsg) tea.Cmd {
	page := max(m.height, 1)
	switch {
	case key.Matches(msg, m.keyMap.Up):
		m.sel--
	case key.Matches(msg, m.keyMap.Down):
		m.sel++
	case key.Matches(msg, m.keyMap.PageUp):
		m.sel -= page
	case key.Matches(msg, m.keyMap.PageDown):
		m.sel += page
	case key.Matches(msg, m.keyMap.Home):
		m.sel = 0
	case key.Matches(msg, m.keyMap.End):
		// Loading the whole history could take many requests, so go to the
		// last commit loaded, which fetches the next chunk.
		m.sel = len(m.rows) - 1
	case key.Matches(msg, m.keyMap.Choose):
		return m.choose()
	case key.Matches(msg, m.keyMap.Retry):
		return m.startFetch()
	default:
		return nil
	}
	return m.sync()
}

func (m Model) choose() tea.Cmd {
	c, ok := m.Selected()
	if !ok {
		return nil
	}
	id := m.id
	return func() tea.Msg { return ChosenMsg{ID: id, Commit: c} }
}

// announce returns a SelectMsg if the cursor is on another commit than the
// last one announced.
func (m *Model) announce() tea.Cmd {
	c, ok := m.Selected()
	if !ok || c.ID == m.announced {
		return nil
	}
	m.announced = c.ID
	id := m.id
	return func() tea.Msg { return SelectMsg{ID: id, Commit: c} }
}

// receive lays out a fetched chunk below the commits loaded so far. Results
// for a chunk that is not the next one are dropped.
func (m *Model) receive(msg chunkMsg) tea.Cmd {
	if !m.fetching || msg.cursor != m.cursor {
		return nil
	}
	m.fetching = false
	if msg.err != nil {
		m.err = msg.err
		m.refreshError()
		return nil
	}

	// One backing array for the cells of the chunk saves an allocation for
	// every row.
	cells := make([]cell, 0, len(msg.commits)*(len(m.layout.lanes)+2))
	seen := func(id string) bool {
		_, ok := m.seen[id]
		return ok
	}
	for i := range msg.commits {
		c := &msg.commits[i]
		if c.ID == "" || seen(c.ID) {
			continue
		}
		m.seen[c.ID] = struct{}{}
		start := len(cells)
		cells = m.layout.add(*c, seen, cells)
		r := row{commit: *c, cells: cells[start:len(cells):len(cells)]}
		m.render(&r)
		m.rows = append(m.rows, r)
	}
	m.cursor = msg.next
	m.done = msg.next == ""
	return m.sync()
}

// sync moves the window to the cursor and fetches the next chunk when the
// window is not full or the cursor nears the last commit loaded.
func (m *Model) sync() tea.Cmd {
	m.scroll()
	if m.done || m.fetching || m.err != nil {
		return nil
	}
	margin := m.prefetch
	if margin == 0 {
		margin = m.height
	}
	if m.top+m.height < len(m.rows) && m.sel+margin < len(m.rows)-1 {
		return nil
	}
	cmd := m.startFetch()
	// Bring the loading row into view.
	m.scroll()
	return cmd
}

// scroll keeps the cursor in range and in view.
func (m *Model) scroll() {
	n := len(m.rows)
	m.sel = max(min(m.sel, n-1), 0)

	slots := max(m.height, 1)
	rows := n
	if m.hasStatus() {
		rows++
	}
	// Fill the window when it grows, rather than leaving space at the end.
	m.top = max(min(m.top, rows-slots), 0)
	m.top = min(m.top, m.sel)
	if m.sel >= m.top+slots {
		m.top = m.sel - slots + 1
	}
	// On the last commit, show the loading or error row below it too.
	if m.sel == n-1 && rows > n {
		m.top = max(m.top, rows-slots)
	}
}

// hasStatus reports whether a loading, error or empty row follows the
// commits.
func (m Model) hasStatus() bool {
	return m.fetching || m.err != nil || len(m.rows) == 0
}
