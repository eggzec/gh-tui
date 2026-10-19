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
