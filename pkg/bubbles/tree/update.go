package tree

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Update handles keys while focused, and the tree's own load results and
// spinner ticks. It ignores messages meant for other trees.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case childrenMsg:
		if msg.tree != m.id {
			return m, nil
		}
		cmd := m.receive(msg)
		return m, cmd
	case spinner.TickMsg:
		if msg.ID != m.spin.ID() {
			return m, nil
		}
		if m.loads == 0 {
			m.spinning = false
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if !m.focused {
			return m, nil
		}
		cmd := m.press(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) press(msg tea.KeyPressMsg) tea.Cmd {
	page := max(m.height, 1)
	// The user took over from a Reveal.
	m.goal = nil
	if key.Matches(msg, m.keyMap.Expand, m.keyMap.Collapse, m.keyMap.Open) {
		m.capped = false
	}
	var cmd tea.Cmd
	switch {
	case key.Matches(msg, m.keyMap.Up):
		m.sel--
	case key.Matches(msg, m.keyMap.Down):
		m.sel++
	case key.Matches(msg, m.keyMap.PageUp):
		m.sel -= page
	case key.Matches(msg, m.keyMap.PageDown):
		m.sel += page
	case key.Matches(msg, m.keyMap.HalfPageUp):
		m.sel -= max(page/2, 1)
	case key.Matches(msg, m.keyMap.HalfPageDown):
		m.sel += max(page/2, 1)
	case key.Matches(msg, m.keyMap.Home):
		m.sel = 0
	case key.Matches(msg, m.keyMap.End):
		m.sel = len(m.rows) - 1
	case key.Matches(msg, m.keyMap.Expand):
		cmd = m.right()
	case key.Matches(msg, m.keyMap.Collapse):
		m.collapse()
	case key.Matches(msg, m.keyMap.ToggleAll):
		cmd = m.toggleAll()
	case key.Matches(msg, m.keyMap.Open):
		cmd = m.open()
	default:
		return nil
	}
	m.scroll()
	return cmd
}

// expand expands e and loads its children if they are not loaded yet. It
// retries a failed load, and a failed load of the top-level nodes first.
func (m *Model) expand(e *entry) tea.Cmd {
	if root := m.nodes[""]; root.err != nil {
		cmd := m.startLoad(root)
		m.flatten(m.anchor(e))
		return cmd
	}
	if e == nil || !e.node.Branch {
		return nil
	}
	e.expanded = true
	var cmd tea.Cmd
	if e.err != nil || !e.loaded && !e.loading {
		cmd = m.startLoad(e)
	}
	m.flatten(m.anchor(e))
	return cmd
}

// Retry loads again what failed to load and is open: the top-level nodes,
// or else each expanded branch, as expanding it does, such as once the
// network is back. It returns nil if nothing open failed.
func (m *Model) Retry() tea.Cmd {
	if root := m.nodes[""]; root != nil && root.err != nil {
		cmd := m.startLoad(root)
		m.flatten(m.anchor(m.current()))
		return cmd
	}
	var cmds []tea.Cmd
	for _, e := range m.nodes {
		if e.err != nil && e.expanded {
			cmds = append(cmds, m.startLoad(e))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	m.flatten(m.anchor(m.current()))
	return tea.Batch(cmds...)
}

// right moves into an expanded branch, expands it, or opens a leaf.
func (m *Model) right() tea.Cmd {
	e := m.current()
	if e != nil && !e.node.Branch {
		return m.open()
	}
	if e != nil && e.expanded && e.err == nil && len(e.kids) > 0 {
		// The first child is the next row.
		m.sel++
		return nil
	}
	return m.expand(e)
}

// collapse collapses the branch under the cursor, or moves to the parent.
func (m *Model) collapse() {
	e := m.current()
	switch {
	case e == nil:
	case e.node.Branch && e.expanded:
		e.expanded = false
		m.flatten(m.anchor(e))
	default:
		for i := m.sel - 1; i >= 0; i-- {
			if m.rows[i].depth < e.depth {
				m.sel = i
				return
			}
		}
	}
}

// toggleAll collapses every branch when all are expanded, when an
// expand-all stopped at its limits, or while one is still loading, which it
// stops, and expands every branch otherwise. A tree without branches stays as it is.
func (m *Model) toggleAll() tea.Cmd {
	branches, all := false, true
	for _, e := range m.nodes {
		if e.depth >= 0 && e.node.Branch {
			branches = true
			all = all && e.expanded
		}
	}
	all = all || m.bulk.active() || m.capped
	switch {
	case !branches:
		return nil
	case all:
		m.collapseAll()
		return nil
	}
	return m.expandAll()
}

// collapseAll collapses every branch and moves the cursor to the top-level
// node it was under. It stops an expand-all in progress.
func (m *Model) collapseAll() {
	m.bulk = bulk{}
	m.capped = false
	sel := m.current()
	for sel != nil && sel.depth > 0 {
		sel = m.nodes[sel.parent]
	}
	for _, e := range m.nodes {
		if e.depth >= 0 {
			e.expanded = false
		}
	}
	m.flatten(m.anchor(sel))
}

// open asks the parent to open the leaf under the cursor, or toggles the
// branch.
func (m *Model) open() tea.Cmd {
	e := m.current()
	switch {
	case e == nil:
		return nil
	case e.node.Branch && e.expanded:
		e.expanded = false
		m.flatten(m.anchor(e))
		return nil
	case e.node.Branch:
		return m.expand(e)
	}
	msg := OpenMsg{ID: m.id, Node: e.node}
	return func() tea.Msg { return msg }
}

// receive stores loaded children. Results nobody waits for any more, for
// example because the node was loaded again or dropped, are ignored.
func (m *Model) receive(msg childrenMsg) tea.Cmd {
	e := m.nodes[msg.node]
	if e == nil || !e.loading || e.seq != msg.seq {
		return nil
	}
	anchor := m.anchor(m.current())
	e.cancel()
	e.cancel, e.loading = nil, false
	m.loads--
	if msg.err != nil {
		m.setErr(e, msg.err)
	} else {
		m.setKids(e, msg.kids)
	}
	cmd := m.settle(e)
	m.flatten(anchor)
	if len(m.goal) > 0 {
		cmd = tea.Batch(cmd, m.advance())
	}
	return cmd
}
