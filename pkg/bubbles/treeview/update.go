package treeview

import (
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// OpenMsg is sent when enter is pressed on a row, whatever the row is: enter
// never folds.
type OpenMsg struct {
	// Tree is the [Model.ID] of the tree that sent it.
	Tree int
	// ID is the ID of the row, and Node the row itself.
	ID   string
	Node Node
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

// Init returns the command that loads the top level, or nil when the source
// has it at once or never loads.
func (m Model) Init() tea.Cmd { return m.lz.boot }

// Update handles the results of loads and, while the tree is focused, keys.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case childrenMsg:
		cmd := m.receive(msg)
		return m, cmd
	case tickMsg:
		cmd := m.onTick(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if !m.focused || len(m.vis) == 0 {
			return m, nil
		}
		cmd := m.press(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) press(msg tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	page := max(m.inner(), 1)
	prev := m.cur
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
		cmd = m.expand()
	case keymap.Matches(msg, m.keyMap.Collapse):
		m.collapse()
	case keymap.Matches(msg, m.keyMap.ToggleAll):
		cmd = m.toggleAll()
	case keymap.Matches(msg, m.keyMap.Open):
		cmd = m.report()
	case keymap.Matches(msg, m.keyMap.Mark):
		cmd = m.mark()
	default:
		return nil
	}
	if m.cur != prev && !m.bottom && m.tall() {
		m.off = m.starts[m.cur]
	}
	m.scroll()
	return cmd
}

// tall reports whether the row under the cursor has more lines than the
// window shows of rows.
func (m Model) tall() bool { return m.lines(m.cur) > max(m.inner(), 1) }

// readRow moves the window lines lines inside the row under the cursor when
// it is taller than the window and has more to show that way, and reports
// whether it did. A negative lines is up.
func (m *Model) readRow(lines int) bool {
	if !m.tall() {
		return false
	}
	s, e := m.starts[m.cur], m.starts[m.cur+1]
	switch {
	case lines > 0 && e > m.off+m.inner():
		m.off = min(m.off+lines, e-m.inner())
	case lines < 0 && s < m.off:
		m.off = max(m.off+lines, s)
	default:
		return false
	}
	return true
}

// step moves the cursor one row in direction dir, over rows it may not rest
// on, and stays where it is at the ends. Inside a row taller than the window
// it scrolls a line first, and a row it enters from below shows its end.
func (m *Model) step(dir int) {
	if m.readRow(dir) {
		return
	}
	for j := m.cur + dir; j >= 0 && j < len(m.vis); j += dir {
		if !m.row(j).Skip {
			m.setCur(j)
			m.bottom = dir < 0
			return
		}
	}
}

// jump moves the cursor lines lines down, or up when negative, to the row
// that has that line, and never the wrong way. Inside a row taller than the
// window it scrolls first, and leaves the row only once its end, or its top
// going up, is in view.
func (m *Model) jump(lines int) {
	if m.readRow(lines) {
		return
	}
	from := m.starts[m.cur]
	if lines > 0 && m.tall() {
		from = max(from, m.starts[m.cur+1]-lines)
	}
	target := min(max(from+lines, 0), m.starts[len(m.vis)]-1)
	j := sort.Search(len(m.vis), func(i int) bool { return m.starts[i+1] > target })
	if lines > 0 {
		m.setCur(max(m.selectable(j, 1), m.cur))
		return
	}
	prev := m.cur
	m.setCur(min(m.selectable(j, -1), m.cur))
	m.bottom = m.cur != prev
}

// expand opens a closed fold and loads its children if they are not there, or
// moves to the first child of an open one. A fold whose load failed loads
// again.
func (m *Model) expand() tea.Cmd {
	r := m.row(m.cur)
	if !m.foldAt(m.cur) {
		return nil
	}
	ri := m.vis[m.cur]
	if m.closed[r.ID] {
		m.closed = withKey(m.closed, r.ID, false)
		m.reflow()
		return m.fetch(ri)
	}
	if m.wantsLoad(ri) {
		return m.fetch(ri)
	}
	for j := m.cur + 1; j < len(m.vis) && m.row(j).Depth > r.Depth; j++ {
		if !m.row(j).Skip {
			m.setCur(j)
			return nil
		}
	}
	return nil
}

// collapse closes an open fold, or moves to the nearest ancestor the cursor
// may rest on.
func (m *Model) collapse() {
	r := m.row(m.cur)
	if m.foldAt(m.cur) && !m.closed[r.ID] {
		m.closed = withKey(m.closed, r.ID, true)
		m.reflow()
		return
	}
	need := r.Depth
	for j := m.cur - 1; j >= 0 && need > 0; j-- {
		p := m.row(j)
		if p.Depth >= need {
			continue
		}
		need = p.Depth
		if !p.Skip {
			m.setCur(j)
			return
		}
	}
}

// toggleAll opens every fold when a fold that shows is closed, else closes
// every fold.
// Opening loads what the branches below have not loaded, within the limits of
// the tree.
func (m *Model) toggleAll() tea.Cmd {
	folds, shut := false, false
	for i := range m.vis {
		if m.foldAt(i) {
			folds = true
			shut = shut || m.closed[m.row(i).ID]
		}
	}
	if !folds {
		return nil
	}
	if shut {
		return m.expandAll()
	}
	m.lz.bulk = bulk{}
	closed := make(map[string]bool, len(m.closed))
	for id := range m.closed {
		closed[id] = true
	}
	m.closed = closed
	m.reflow()
	return nil
}

// report sends the row under the cursor as an OpenMsg. It never changes a
// fold.
func (m *Model) report() tea.Cmd {
	r := m.row(m.cur)
	id := m.id
	return func() tea.Msg { return OpenMsg{Tree: id, ID: r.ID, Node: r} }
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
