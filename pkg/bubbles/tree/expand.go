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

// expandAll expands the branch under the cursor and every branch below it,
// within the limits set by [WithExpandAllLimits].
func (m *Model) expandAll() tea.Cmd {
	e := m.current()
	if e == nil || !e.node.Branch {
		return nil
	}
	m.bulk = bulk{queue: []string{e.node.ID}, base: e.depth, waiting: map[string]bool{}}
	cmd := m.pump()
	m.flatten(m.anchor(e))
	return cmd
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
