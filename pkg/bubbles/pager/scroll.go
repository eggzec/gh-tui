package pager

import (
	"github.com/eggzec/gh-tui/pkg/bubbles/internal/wrap"
)

// bodyHeight returns the rows above the status line.
func (m Model) bodyHeight() int { return max(m.height-1, 0) }

// gutterWidth returns the width of the line numbers and the space after
// them, or of the marks of an inverted search while the numbers are
// hidden, or 0 when there is neither or no room for text.
func (m Model) gutterWidth() int {
	if m.gutter != nil {
		return m.customWidth()
	}
	return numberGutter{m: &m}.Width(m.width)
}

// customWidth returns the width of the host's gutter, or 0 when it leaves
// no room for text.
func (m Model) customWidth() int {
	if w := m.gutter.Width(m.width); w > 0 && w < m.width {
		return w
	}
	return 0
}

// reserved returns how many lines more than the content has the gutter
// has room to number, for rendered content that may grow.
func (m Model) reserved() int {
	if m.render == nil || m.reserve == nil {
		return 0
	}
	return max(m.reserve(), 0)
}

// gutterFor returns the width of the gutter of n lines, as gutterWidth
// does of the lines of the content.
func (m Model) gutterFor(n int) int {
	if m.gutter != nil {
		return m.customWidth()
	}
	return numberGutter{m: &m}.widthFor(n, m.width)
}

// textWidth returns the columns right of the gutter.
func (m Model) textWidth() int { return max(m.width-m.gutterWidth(), 0) }

// indent returns the blank cells before the text of row i, which the rows
// ask for with [Indenter], or 0. It leaves at least one column for the text.
func (m Model) indent(i int) int {
	in, ok := m.rows.(Indenter)
	if !ok {
		return 0
	}
	return min(max(in.Indent(i), 0), max(m.textWidth()-1, 0))
}

// widthOf returns the columns of the text of row i, right of the gutter and
// its indent.
func (m Model) widthOf(i int) int { return max(m.textWidth()-m.indent(i), 0) }

// rowsIn returns how many rows the line shown at p takes.
func (m Model) rowsIn(p int) int {
	if !m.wrap {
		return 1
	}
	if _, ok := m.header(m.at(p)); ok {
		return 1
	}
	s, tw := m.rows.Text(m.at(p)), m.widthOf(m.at(p))
	n := 1
	for j, _ := wrap.NextRow(s, 0, tw); j < len(s); j, _ = wrap.NextRow(s, j, tw) {
		n++
	}
	return n
}

// rowOf returns the row of the line shown at p that holds byte b.
func (m Model) rowOf(p, b int) int {
	if _, ok := m.header(m.at(p)); ok {
		return 0
	}
	s, tw := m.rows.Text(m.at(p)), m.widthOf(m.at(p))
	r := 0
	for j, _ := wrap.NextRow(s, 0, tw); j <= b && j < len(s); j, _ = wrap.NextRow(s, j, tw) {
		r++
	}
	return r
}

// inView reports whether row r of the line shown at p is in the window.
// It walks only the lines above it in the window.
func (m Model) inView(p, r int) bool {
	if p < m.top || p == m.top && r < m.row {
		return false
	}
	h, rows := m.bodyHeight(), -m.row
	for q := m.top; q < p; q++ {
		if rows += m.rowsIn(q); rows >= h {
			return false
		}
	}
	return rows+r < h
}

// down scrolls n rows down.
func (m *Model) down(n int) {
	if !m.wrap || m.count() == 0 {
		m.top += n
		m.clamp()
		return
	}
	for ; n > 0; n-- {
		switch {
		case m.row+1 < m.rowsIn(m.top):
			m.row++
		case m.top+1 < m.count():
			m.top, m.row = m.top+1, 0
		default:
			n = 0
		}
	}
	m.clamp()
}

// up scrolls n rows up.
func (m *Model) up(n int) {
	if !m.wrap || m.count() == 0 {
		m.top -= n
		m.clamp()
		return
	}
	for ; n > 0; n-- {
		switch {
		case m.row > 0:
			m.row--
		case m.top > 0:
			m.top--
			m.row = m.rowsIn(m.top) - 1
		default:
			n = 0
		}
	}
	m.clamp()
}

// last returns the top line and row of the window scrolled to the end.
func (m Model) last() (top, row int) {
	n, h := m.count(), m.bodyHeight()
	if n == 0 || h == 0 {
		return 0, 0
	}
	if !m.wrap {
		return max(n-h, 0), 0
	}
	top, row = n-1, m.rowsIn(n-1)-1
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

// bottom returns the position of the last line shown with a row in the
// window.
func (m Model) bottom() int {
	h, n := m.bodyHeight(), m.count()
	if n == 0 {
		return -1
	}
	if !m.wrap {
		// Never past the last row, even when the window is stale.
		return max(min(m.top+h, n)-1, min(m.top, n-1))
	}
	i, rows := m.top, m.rowsIn(m.top)-m.row
	for rows < h && i+1 < n {
		i++
		rows += m.rowsIn(i)
	}
	return i
}

// GoToLine scrolls line n, counted from 1, into view a third of the way
// down, and marks its number, so that a parent can open the pager on a
// line, such as the one an error points at. A line past the end goes to
// the last. Set it after SetContent; new content clears the mark.
func (m *Model) GoToLine(n int) {
	if m.rows.Len() == 0 || n < 1 {
		return
	}
	i := min(n, m.rows.Len()) - 1
	m.mark = i
	if m.cursoring {
		m.cursor = i
	}
	m.top, m.row = max(m.posOf(i)-m.bodyHeight()/3, 0), 0
	m.clamp()
}

// clamp keeps the window within the content after anything that moved or
// resized it.
func (m *Model) clamp() {
	if m.cursoring {
		m.snapCursor()
	}
	lastTop, lastRow := m.last()
	if m.top > lastTop || m.top == lastTop && m.row > lastRow {
		m.top, m.row = lastTop, lastRow
	}
	m.top = max(m.top, 0)
	switch {
	case !m.wrap || m.count() == 0:
		m.row = 0
	default:
		m.row = max(min(m.row, m.rowsIn(m.top)-1), 0)
	}
	if m.wrap {
		m.left = 0
	}
	m.left = max(m.left, 0)
	m.findHits()
}

// scrollRight scrolls n columns right, but no further than the longest
// line in the window reaches.
func (m *Model) scrollRight(n int) {
	want := m.left + n
	reach := 0
	for p, last := m.top, m.bottom(); p <= last; p++ {
		if _, ok := m.header(m.at(p)); ok {
			// A fold row doesn't scroll sideways.
			continue
		}
		tw := m.widthOf(m.at(p))
		// One more column finds a wide rune that crosses the edge.
		_, w := wrap.Advance(m.rows.Text(m.at(p)), 0, want+tw+1)
		reach = max(reach, w-tw)
	}
	m.left = max(min(want, reach), m.left)
}

// hStep returns the columns one sideways scroll moves. It is the same for
// every row, so it ignores their indents.
func (m Model) hStep() int { return max(m.textWidth()/4, 1) }
