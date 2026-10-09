package tree

import tea "charm.land/bubbletea/v2"

// bulk is an expand-all in progress. It expands branches level by level, so
// the nodes closest to the cursor come first, and keeps at most maxLoads
// loads in flight.
type bulk struct {
	// queue holds the IDs of the branches still to expand, in order.
	queue []string
	// base is the depth of the branch the expand-all started from.
	base int
	// waiting holds the branches whose children are loading. A nil map
	// means no expand-all is in progress.
	waiting map[string]bool
	// revealed counts the children of the branches expanded so far.
	revealed int
}

func (b *bulk) active() bool {
	return b.waiting != nil
}

// unwait stops waiting for the children of the node with the given ID.
func (b *bulk) unwait(id string) {
	delete(b.waiting, id)
}

// expandAll expands every branch, level by level from the top, within the
// limits set by [WithExpandAllLimits].
func (m *Model) expandAll() tea.Cmd {
	root := m.nodes[""]
	if root == nil {
		return nil
	}
	m.capped = false
	m.bulk = bulk{base: 0, waiting: map[string]bool{}}
	for _, k := range root.kids {
		if c := m.nodes[k]; c != nil && c.node.Branch {
			m.bulk.queue = append(m.bulk.queue, k)
		}
	}
	cur := m.current()
	cmd := m.pump()
	m.flatten(m.anchor(cur))
	return cmd
}

// ExpandAll expands every branch, level by level, within the limits of
// [WithExpandAllLimits]: what lies beyond them stays closed. Unlike the
// toggle-all key it never collapses, not even after the limits were hit, so
// a parent that shows a short list in full can ask again after each
// [Model.Reload] or [Model.ReloadNode]. While any load is in flight, such
// as the top-level nodes' first, it waits until they are done, so that
// the branches they bring are expanded too.
func (m *Model) ExpandAll() tea.Cmd {
	if m.loads > 0 {
		m.expandOnLoad = true
		return nil
	}
	return m.expandAll()
}

// pump expands queued branches until maxLoads loads are in flight, the queue
// is empty, or the node budget is spent.
func (m *Model) pump() tea.Cmd {
	b := &m.bulk
	var cmds []tea.Cmd
	for len(b.queue) > 0 && len(b.waiting) < m.maxLoads && b.revealed < m.expandNodes {
		e := m.nodes[b.queue[0]]
		b.queue = b.queue[1:]
		if e == nil || !e.node.Branch {
			continue
		}
		e.expanded = true
		switch {
		case e.loaded:
			m.reveal(e)
		case e.loading:
			b.waiting[e.node.ID] = true
		default:
			cmds = append(cmds, m.startLoad(e))
			b.waiting[e.node.ID] = true
		}
	}
	if b.revealed >= m.expandNodes {
		m.capped = m.capped || len(b.queue) > 0
		b.queue = nil
	}
	if len(b.queue) == 0 && len(b.waiting) == 0 {
		m.bulk = bulk{}
	}
	return tea.Batch(cmds...)
}

// reveal counts the children of e and queues its branches, unless they are
// deeper than the expand-all may go.
func (m *Model) reveal(e *entry) {
	b := &m.bulk
	b.revealed += len(e.kids)
	if e.depth+1-b.base >= m.expandDepth {
		for _, k := range e.kids {
			if c := m.nodes[k]; c != nil && c.node.Branch {
				m.capped = true
			}
		}
		return
	}
	for _, k := range e.kids {
		if c := m.nodes[k]; c != nil && c.node.Branch {
			b.queue = append(b.queue, k)
		}
	}
}

// settle continues an expand-all after e finished loading.
func (m *Model) settle(e *entry) tea.Cmd {
	if !m.bulk.active() {
		return nil
	}
	if m.bulk.waiting[e.node.ID] {
		m.bulk.unwait(e.node.ID)
		if e.err == nil {
			m.reveal(e)
		}
	}
	return m.pump()
}
