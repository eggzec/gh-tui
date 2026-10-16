package foldtree

import (
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// EnterMsg is sent when enter is pressed on a row that has an action of its
// own, or on a row that does not fold.
type EnterMsg struct {
	// Tree is the [Model.ID] of the tree that sent it.
	Tree int
	// ID is the ID of the row.
	ID string
}

// MarkMsg is sent when space is pressed on a row that has a kind. The
// tree moves its cursor to the next row of that kind, and leaves it to the
// caller to flip the mark of the row it names.
type MarkMsg struct {
	// Tree is the [Model.ID] of the tree that sent it.
	Tree int
	// ID is the ID of the row that was marked, and Kind its kind.
	ID, Kind string
}

// NoMarkMsg is sent when space is pressed on a row that has no kind, so that
// the caller can say what space works on.
type NoMarkMsg struct {
	// Tree is the [Model.ID] of the tree that sent it.
	Tree int
	// ID is the ID of the row.
	ID string
}

// Init returns nil: a tree has nothing to start.
func (m Model) Init() tea.Cmd { return nil }

// Update handles keys while the tree is focused.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.focused || len(m.vis) == 0 {
		return m, nil
	}
	cmd := m.press(press)
	return m, cmd
}

func (m *Model) press(msg tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	page := max(m.inner(), 1)
	switch {
	case keymap.Matches(msg, m.keyMap.Up):
		m.step(-1)
	case keymap.Matches(msg, m.keyMap.Down):
		m.step(1)
	case keymap.Matches(msg, m.keyMap.PageUp):
		m.jump(-page)
	case keymap.Matches(msg, m.keyMap.PageDown):
		m.jump(page)
	case keymap.Matches(msg, m.keyMap.HalfPageUp):
		m.jump(-max(page/2, 1))
	case keymap.Matches(msg, m.keyMap.HalfPageDown):
		m.jump(max(page/2, 1))
	case keymap.Matches(msg, m.keyMap.Home):
		m.setCur(m.selectable(0, 1))
	case keymap.Matches(msg, m.keyMap.End):
		m.setCur(m.selectable(len(m.vis)-1, -1))
	case keymap.Matches(msg, m.keyMap.Expand):
		m.expand()
	case keymap.Matches(msg, m.keyMap.Collapse):
		m.collapse()
	case keymap.Matches(msg, m.keyMap.ToggleAll):
		m.toggleAll()
	case keymap.Matches(msg, m.keyMap.Enter):
		cmd = m.enter()
	case keymap.Matches(msg, m.keyMap.Mark):
		cmd = m.mark()
	default:
		return nil
	}
	m.scroll()
	return cmd
}

// step moves the cursor one row in direction dir, over rows it may not rest
// on, and stays where it is at the ends.
func (m *Model) step(dir int) {
	for j := m.cur + dir; j >= 0 && j < len(m.vis); j += dir {
		if !m.row(j).Skip {
			m.setCur(j)
			return
		}
	}
}

// jump moves the cursor lines lines down, or up when negative, to the row
// that has that line, and never the wrong way.
func (m *Model) jump(lines int) {
	target := min(max(m.starts[m.cur]+lines, 0), m.starts[len(m.vis)]-1)
	j := sort.Search(len(m.vis), func(i int) bool { return m.starts[i+1] > target })
	if lines > 0 {
		m.setCur(max(m.selectable(j, 1), m.cur))
		return
	}
	m.setCur(min(m.selectable(j, -1), m.cur))
}

// expand opens a closed fold, or moves to the first child of an open one.
func (m *Model) expand() {
	r := m.row(m.cur)
	if !r.Fold {
		return
	}
	if m.closed[r.ID] {
		m.closed = withKey(m.closed, r.ID, false)
		m.reflow()
		return
	}
	for j := m.cur + 1; j < len(m.vis) && m.row(j).Depth > r.Depth; j++ {
		if !m.row(j).Skip {
			m.setCur(j)
			return
		}
	}
}

// collapse closes an open fold, or moves to the nearest ancestor the cursor
// may rest on.
func (m *Model) collapse() {
	r := m.row(m.cur)
	if r.Fold && !m.closed[r.ID] {
		m.closed = withKey(m.closed, r.ID, true)
		m.reflow()
		return
	}
	for j := m.cur - 1; j >= 0; j-- {
		if p := m.row(j); p.Depth < r.Depth && !p.Skip {
			m.setCur(j)
			return
		}
	}
}

// toggleAll opens every fold when a fold that shows is closed, else closes
// every fold.
func (m *Model) toggleAll() {
	folds, shut := false, false
	for i := range m.vis {
		if r := m.row(i); r.Fold {
			folds = true
			shut = shut || m.closed[r.ID]
		}
	}
	if !folds {
		return
	}
	closed := make(map[string]bool, len(m.closed))
	for id := range m.closed {
		closed[id] = !shut
	}
	m.closed = closed
	m.reflow()
}

// enter toggles a fold that has no action, and reports any other row.
func (m *Model) enter() tea.Cmd {
	r := m.row(m.cur)
	if r.Fold && !r.Action {
		m.closed = withKey(m.closed, r.ID, !m.closed[r.ID])
		m.reflow()
		return nil
	}
	id := m.id
	return func() tea.Msg { return EnterMsg{Tree: id, ID: r.ID} }
}

// mark reports the row under the cursor and moves to the next row of its
// kind in display order, opening the folds above it.
func (m *Model) mark() tea.Cmd {
	r := m.row(m.cur)
	id := m.id
	if r.Kind == "" {
		return func() tea.Msg { return NoMarkMsg{Tree: id, ID: r.ID} }
	}
	for j := m.vis[m.cur] + 1; j < len(m.rows); j++ {
		if n := m.rows[j]; n.Kind == r.Kind && !n.Skip {
			m.open(j)
			m.reflow()
			m.setCur(m.visIndex(n.ID))
			break
		}
	}
	return func() tea.Msg { return MarkMsg{Tree: id, ID: r.ID, Kind: r.Kind} }
}
