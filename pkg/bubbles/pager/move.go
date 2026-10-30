package pager

// The moves below are the ones the keys make, for a host that draws the
// body itself and takes its own keys. They move the cursor when the pager
// has one, and the window otherwise.

// Down moves n rows down: the cursor, or the window without one.
func (m *Model) Down(n int) { m.scroll(max(n, 0)) }

// Up moves n rows up: the cursor, or the window without one.
func (m *Model) Up(n int) { m.scroll(-max(n, 0)) }

// PageDown moves a page down.
func (m *Model) PageDown() { m.scroll(max(m.bodyHeight(), 1)) }

// PageUp moves a page up.
func (m *Model) PageUp() { m.scroll(-max(m.bodyHeight(), 1)) }

// HalfPageDown moves half a page down.
func (m *Model) HalfPageDown() { m.scroll(max(m.bodyHeight()/2, 1)) }

// HalfPageUp moves half a page up.
func (m *Model) HalfPageUp() { m.scroll(-max(m.bodyHeight()/2, 1)) }

// Home moves to the first line.
func (m *Model) Home() {
	if m.cursoring {
		m.cursorHome()
		return
	}
	m.top, m.row = 0, 0
	m.clamp()
}

// End moves to the last line.
func (m *Model) End() {
	if m.cursoring {
		m.cursorEnd()
		return
	}
	m.top, m.row = m.last()
	m.clamp()
}

// Left scrolls one step left.
func (m *Model) Left() {
	m.left = max(m.left-m.hStep(), 0)
	m.findHits()
}

// Right scrolls one step right, no further than the lines in the window
// reach. Wrapped text doesn't scroll sideways.
func (m *Model) Right() {
	if m.wrap {
		return
	}
	m.scrollRight(m.hStep())
	m.findHits()
}

// Bottom returns the row of the last line with a row in the window, or -1
// when no line is shown.
func (m Model) Bottom() int {
	p := m.bottom()
	if p < 0 {
		return -1
	}
	return m.at(p)
}

// Show scrolls the window as little as it takes to show the cursor.
func (m *Model) Show() { m.show() }

// JumpTo shows the line at position p, a third of the way down when it is
// out of view.
func (m *Model) JumpTo(p int) { m.jumpTo(p) }
