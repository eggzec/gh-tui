package tree

import (
	"slices"

	tea "charm.land/bubbletea/v2"
)

// Reveal expands the branches on the way to a node and moves the cursor to
// it, such as to show a file found by its path. path holds the IDs from a
// top-level node down to the node, each a child of the one before.
//
// Branches that aren't loaded yet load first, and the cursor follows as
// they arrive. If a node on the way is missing, or its branch fails to
// load, the cursor stays on the deepest node found. A key press, or another
// Reveal, gives up the one in progress.
func (m *Model) Reveal(path ...string) tea.Cmd {
	m.goal = slices.Clone(path)
	return m.advance()
}

// advance walks the path of a Reveal as far as the loaded nodes go, expands
// the branches on the way, and starts the load of the first that isn't
// loaded. It forgets the path once the walk ends, and moves the cursor to
// the deepest node it reached.
func (m *Model) advance() tea.Cmd {
	if len(m.goal) == 0 {
		return nil
	}
	parent := m.nodes[""]
	var at *entry
	var cmd tea.Cmd
	done := true
	for i, id := range m.goal {
		if parent.loading {
			done = false
			break
		}
		e := m.nodes[id]
		if e == nil || e.parent != parent.node.ID {
			break
		}
		at = e
		if i == len(m.goal)-1 || !e.node.Branch {
			break
		}
		e.expanded = true
		if !e.loaded && !e.loading {
			if e.err != nil {
				break
			}
			cmd = m.startLoad(e)
		}
		parent = e
	}
	if done {
		m.goal = nil
	}
	if at != nil {
		m.flatten([]string{at.node.ID})
	}
	return cmd
}
