package tree

import (
	tea "charm.land/bubbletea/v2"
)

// open reports whether the branch shows its children: it is expanded, or a
// filter holds it open.
func (e *entry) open() bool {
	return e.expanded || e.forced
}

// Expand expands the branch with the given ID and loads its children if
// they aren't loaded yet, as the expand key does, but without moving the
// cursor. It retries a failed load, and does nothing for an ID that isn't
// a branch of the tree.
func (m *Model) Expand(id string) tea.Cmd {
	e := m.branch(id)
	if e == nil {
		return nil
	}
	anchor := m.anchor(m.current())
	e.expanded = true
	var cmd tea.Cmd
	if e.err != nil || !e.loaded && !e.loading {
		cmd = m.startLoad(e)
	}
	m.flatten(anchor)
	return cmd
}

// Load loads the children of the branch with the given ID without
// expanding it, such as for a filter to match them. It does nothing when
// they are loaded or loading, and retries a failed load.
func (m *Model) Load(id string) tea.Cmd {
	e := m.branch(id)
	if e == nil || e.loaded && e.err == nil || e.loading {
		return nil
	}
	anchor := m.anchor(m.current())
	cmd := m.startLoad(e)
	m.flatten(anchor)
	return cmd
}

// branch returns the branch with the given ID, or nil.
func (m *Model) branch(id string) *entry {
	if e := m.nodes[id]; e != nil && e.depth >= 0 && e.node.Branch {
		return e
	}
	return nil
}

// Rename changes the name of the node with the given ID, such as to show a
// count in the heading of a branch, and keeps everything else about it.
func (m *Model) Rename(id, name string) {
	e := m.nodes[id]
	if e == nil || e.depth < 0 || e.node.Name == name {
		return
	}
	e.node.Name = name
	e.label = m.label(e.node)
}

// SetFilter shows only the leaves that match, as they are loaded, and the
// branches that hold a match, which show expanded without their stored
// state changing: clearing the filter, with nil, puts the branches back as
// they were. A branch with no loaded child that matches is hidden. The
// cursor stays where it is, or moves to the next row shown if its row is
// hidden. Branches whose children aren't loaded are hidden until they
// load; see [Model.Load].
func (m *Model) SetFilter(match func(Node) bool) {
	anchor := m.anchor(m.current())
	m.match = match
	if match == nil {
		for _, e := range m.nodes {
			e.forced = false
		}
	}
	m.flatten(anchor)
}

// Filtered returns how many leaves below the branch with the given ID are
// loaded, and how many of them the filter shows, for a heading such as
// "2 of 5". Without a filter, both are the loaded leaves.
func (m Model) Filtered(id string) (shown, total int) {
	e := m.nodes[id]
	if e == nil {
		return 0, 0
	}
	var count func(e *entry)
	count = func(e *entry) {
		for _, k := range e.kids {
			c := m.nodes[k]
			switch {
			case c == nil:
			case c.node.Branch:
				count(c)
			default:
				total++
				if m.match == nil || m.match(c.node) {
					shown++
				}
			}
		}
	}
	count(e)
	return shown, total
}

// flattenFiltered lists the rows the filter shows, and keeps the cursor on
// the first node of anchor, or else on the next row shown.
func (m *Model) flattenFiltered(anchor []string) {
	m.rows = m.rows[:0]
	var walk func(e *entry) bool
	walk = func(e *entry) bool {
		if !e.node.Branch {
			if !m.match(e.node) {
				return false
			}
			m.rows = append(m.rows, e)
			return true
		}
		at := len(m.rows)
		m.rows = append(m.rows, e)
		shown := false
		for _, k := range e.kids {
			if c := m.nodes[k]; c != nil && walk(c) {
				shown = true
			}
		}
		if !shown {
			m.rows = m.rows[:at]
		}
		e.forced = shown
		return shown
	}
	for _, k := range m.nodes[""].kids {
		if e := m.nodes[k]; e != nil {
			walk(e)
		}
	}
	maxDepth := 0
	at := make(map[*entry]int, len(m.rows))
	for i, e := range m.rows {
		at[e] = i
		maxDepth = max(maxDepth, e.depth)
	}
	m.growGuides(maxDepth)
	switch i, ok := m.rowOf(anchor, at); {
	case ok:
		m.sel = i
	case len(anchor) > 0:
		m.sel = m.nextShown(m.nodes[anchor[0]], at)
	default:
		m.sel = 0
	}
	m.scroll()
}

// rowOf returns the row of the first node of anchor, if it is shown.
func (m *Model) rowOf(anchor []string, at map[*entry]int) (int, bool) {
	if len(anchor) == 0 {
		return 0, false
	}
	i, ok := at[m.nodes[anchor[0]]]
	return i, ok
}

// nextShown returns the row of the first node after from, in the order of
// the whole tree, that is shown, or else of the last one before it.
func (m *Model) nextShown(from *entry, at map[*entry]int) int {
	if len(m.rows) == 0 {
		return 0
	}
	var order []*entry
	var walk func(e *entry)
	walk = func(e *entry) {
		order = append(order, e)
		for _, k := range e.kids {
			if c := m.nodes[k]; c != nil {
				walk(c)
			}
		}
	}
	for _, k := range m.nodes[""].kids {
		if e := m.nodes[k]; e != nil {
			walk(e)
		}
	}
	start := 0
	for i, e := range order {
		if e == from {
			start = i
			break
		}
	}
	for _, e := range order[start:] {
		if i, ok := at[e]; ok {
			return i
		}
	}
	for i := start - 1; i >= 0; i-- {
		if j, ok := at[order[i]]; ok {
			return j
		}
	}
	return 0
}
