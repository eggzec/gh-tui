package pager

import "strings"

// WithCursor gives the pager a cursor: a current row that the line, page,
// top and bottom keys move, and that a search lands on. It is off by
// default, and a pager without it scrolls as before.
func WithCursor(on bool) Option {
	return func(s *settings) { s.cursoring = on }
}

// CursorGutter is an optional interface of a [Gutter]: while the pager has a
// cursor, it draws the cells of a row with WriteCursor in place of Write,
// and says whether the row is the cursor's.
type CursorGutter interface {
	Gutter
	// WriteCursor writes exactly Width cells for row i, as Write does.
	WriteCursor(b *strings.Builder, i int, first, cursor bool)
}

// Position is where a pager is in its rows, to keep through a change of
// document or to restore later. Cursor is a row of the [Rows]; Top and Row
// are the first line of the window among the lines shown, and the first of
// its screen rows when wrapping; Left is the first column when not wrapping.
type Position struct{ Cursor, Top, Row, Left int }

// Cursoring reports whether the pager has a cursor.
func (m Model) Cursoring() bool { return m.cursoring }

// Cursor returns the row the cursor is on, or 0 when the pager has no
// cursor.
func (m Model) Cursor() int {
	if !m.cursoring {
		return 0
	}
	return m.cursor
}

// SetCursor moves the cursor to row i, kept within the rows, and scrolls it
// into view, a third of the way down when it was out of it. A row that a
// filter hides moves it to the next one shown. It does nothing without a
// cursor.
func (m *Model) SetCursor(i int) {
	if !m.cursoring || m.rows.Len() == 0 {
		return
	}
	m.cursor = min(max(i, 0), m.rows.Len()-1)
	m.clamp()
	m.jumpTo(m.curPos())
}

// Position returns where the pager is.
func (m Model) Position() Position {
	return Position{Cursor: m.Cursor(), Top: m.top, Row: m.row, Left: m.left}
}

// SetPosition restores a position that Position returned, kept within the
// rows. It leaves the window where p puts it, even if the cursor is out of
// it.
func (m *Model) SetPosition(p Position) {
	m.top, m.row, m.left = p.Top, p.Row, p.Left
	if m.cursoring {
		m.cursor = max(p.Cursor, 0)
	}
	m.clamp()
}

// curPos returns the position of the cursor among the lines shown.
func (m *Model) curPos() int {
	return max(min(m.posOf(m.cursor), m.count()-1), 0)
}

// setCurPos puts the cursor on the line shown at position p, kept within
// the lines shown.
func (m *Model) setCurPos(p int) {
	if n := m.count(); n > 0 {
		m.cursor = m.at(max(min(p, n-1), 0))
	}
}

// snapCursor keeps the cursor on a line shown: the row that a filter hides
// moves it to the next one shown, or the last.
func (m *Model) snapCursor() {
	if m.count() == 0 {
		m.cursor = 0
		return
	}
	m.setCurPos(m.curPos())
}

// covers reports whether all the rows of the line shown at p are in the
// window.
func (m *Model) covers(p int) bool {
	return m.inView(p, 0) && m.inView(p, m.rowsIn(p)-1)
}

// endAt returns the top line and row of the window that ends with the last
// row of the line shown at p.
func (m Model) endAt(p int) (top, row int) {
	h := m.bodyHeight()
	if !m.wrap {
		return max(p-h+1, 0), 0
	}
	top, row = p, m.rowsIn(p)-1
	for range h - 1 {
		switch {
		case row > 0:
			row--
		case top > 0:
			top--
			row = m.rowsIn(top) - 1
		default:
			return top, row
		}
	}
	return top, row
}

// show scrolls the window as little as it takes to show the cursor.
func (m *Model) show() {
	if m.count() == 0 || m.bodyHeight() == 0 {
		return
	}
	p := m.curPos()
	if m.covers(p) {
		return
	}
	if p <= m.top {
		m.top, m.row = p, 0
		m.clamp()
		return
	}
	top, row := m.endAt(p)
	if top > p || top == p && row > 0 {
		// Taller than the window: show its first row.
		top, row = p, 0
	}
	m.top, m.row = top, row
	m.clamp()
}

// jumpTo shows the line at p, a third of the way down when it is out of
// view, with what leads to it above.
func (m *Model) jumpTo(p int) {
	if m.count() == 0 || m.covers(p) {
		return
	}
	m.top, m.row = max(p-m.bodyHeight()/3, 0), 0
	m.clamp()
	m.show()
}

// moveCursor moves the cursor n lines, scrolling as little as it takes.
func (m *Model) moveCursor(n int) {
	m.setCurPos(m.curPos() + n)
	m.show()
}

// pageCursor scrolls the window n rows, down for a positive n, and moves
// the cursor as many lines, keeping it in the window.
func (m *Model) pageCursor(n int) {
	p := m.curPos() + n
	if n > 0 {
		m.down(n)
	} else {
		m.up(-n)
	}
	m.setCurPos(p)
	m.setCurPos(max(min(m.curPos(), m.bottom()), m.top))
	m.show()
}

// cursorHome moves the cursor and the window to the start.
func (m *Model) cursorHome() {
	m.top, m.row = 0, 0
	m.setCurPos(0)
	m.clamp()
}

// cursorEnd moves the cursor and the window to the end.
func (m *Model) cursorEnd() {
	m.top, m.row = m.last()
	m.setCurPos(m.count() - 1)
	m.clamp()
}

// cursorAtEnd reports whether the cursor is on the last line shown.
func (m Model) cursorAtEnd() bool { return m.curPos() >= m.count()-1 }
