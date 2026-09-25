package logview

import (
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// advance walks the graphemes of the plain text s from byte i, as long as
// they fit in cols cells. It returns the byte where it stopped and the
// cells it walked. Unlike ansi.Truncate it stops there, so walking a window
// of a long line costs the window, not the line. It is the pager's walker.
func advance(s string, i, cols int) (j, used int) {
	for j = i; j < len(s); {
		// ASCII followed by ASCII is a grapheme of its own; sanitize has
		// removed the control characters.
		if s[j] < utf8.RuneSelf && (j+1 == len(s) || s[j+1] < utf8.RuneSelf) {
			if used+1 > cols {
				return j, used
			}
			used++
			j++
			continue
		}
		g, w := ansi.FirstGraphemeCluster(s[j:], ansi.GraphemeWidth)
		if used+w > cols {
			return j, used
		}
		used += w
		j += len(g)
	}
	return j, used
}

// nextRow returns where the wrapped row of s that starts at byte i ends,
// and its width. A row holds at least one grapheme, so a grapheme wider
// than cols makes a row wider than cols.
func nextRow(s string, i, cols int) (j, used int) {
	j, used = advance(s, i, cols)
	if j == i && i < len(s) {
		g, w := ansi.FirstGraphemeCluster(s[i:], ansi.GraphemeWidth)
		return i + len(g), w
	}
	return j, used
}

// minText is the fewest columns of text the gutter leaves; the times and
// then the line numbers are hidden to keep them.
const minText = 20

// timeWidth is the width of a time in the gutter.
const timeWidth = 8

// layout is the widths of the gutter: the cursor, the line numbers, the
// mark of errors and warnings, and the times. A width of 0 hides a part;
// the others include the space after them.
type layout struct {
	numbers, times int
}

// layout returns the widths of the parts of the gutter at the current
// width.
func (m *Model) layout() layout {
	var l layout
	if m.lineNumbers && m.n > 0 {
		l.numbers = digits(m.n) + 1
	}
	if m.times != TimeHidden {
		l.times = timeWidth + 1
	}
	if l.width()+minText > m.width {
		l.times = 0
	}
	if l.width()+minText > m.width {
		l.numbers = 0
	}
	return l
}

// digits returns the number of decimal digits of n > 0.
func digits(n int) int {
	d := 1
	for ; n >= 10; n /= 10 {
		d++
	}
	return d
}

// width returns the width of the gutter: the cursor, the numbers, the mark
// and a space, and the times.
func (l layout) width() int { return 1 + l.numbers + 2 + l.times }

// indent returns the cells before the text of row r: two for each fold it
// is in, and two for the marker of a fold it starts.
func indent(r *row) int {
	n := 2 * r.depth
	if r.fold >= 0 {
		n += 2
	}
	return n
}

// textWidth returns the columns of text of row r.
func (m *Model) textWidth(l layout, r *row) int {
	return max(m.width-l.width()-indent(r), 0)
}

// bodyHeight returns the rows above the status line.
func (m *Model) bodyHeight() int { return max(m.height-1, 0) }

// rowsIn returns how many rows entry v of vis takes.
func (m *Model) rowsIn(v int) int {
	r := &m.rows[m.vis[v]]
	if !m.wrap || r.fold >= 0 {
		return 1
	}
	tw := m.textWidth(m.layout(), r)
	if tw == 0 {
		return 1
	}
	n := 1
	for j, _ := nextRow(r.text, 0, tw); j < len(r.text); j, _ = nextRow(r.text, j, tw) {
		n++
	}
	return n
}

// rowOf returns the row of entry v that holds byte b of its text.
func (m *Model) rowOf(v, b int) int {
	r := &m.rows[m.vis[v]]
	tw := m.textWidth(m.layout(), r)
	if !m.wrap || r.fold >= 0 || tw == 0 {
		return 0
	}
	n := 0
	for j, _ := nextRow(r.text, 0, tw); j <= b && j < len(r.text); j, _ = nextRow(r.text, j, tw) {
		n++
	}
	return n
}

// down scrolls the window n rows down.
func (m *Model) down(n int) {
	if !m.wrap || len(m.vis) == 0 {
		m.top += n
		m.clamp()
		return
	}
	for ; n > 0; n-- {
		switch {
		case m.row+1 < m.rowsIn(m.top):
			m.row++
		case m.top+1 < len(m.vis):
			m.top, m.row = m.top+1, 0
		default:
			n = 0
		}
	}
	m.clamp()
}

// up scrolls the window n rows up.
func (m *Model) up(n int) {
	if !m.wrap || len(m.vis) == 0 {
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

// endAt returns the top entry and row of the window whose last row is the
// last row of entry v.
func (m *Model) endAt(v int) (top, row int) {
	h := m.bodyHeight()
	if len(m.vis) == 0 || h == 0 {
		return 0, 0
	}
	if !m.wrap {
		return max(v-h+1, 0), 0
	}
	top, row = v, m.rowsIn(v)-1
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

// last returns the top entry and row of the window scrolled to the end.
func (m *Model) last() (top, row int) { return m.endAt(len(m.vis) - 1) }

// bottom returns the last entry with a row in the window.
func (m *Model) bottom() int {
	h := m.bodyHeight()
	if len(m.vis) == 0 {
		return -1
	}
	if !m.wrap {
		return max(min(m.top+h, len(m.vis))-1, m.top)
	}
	v, rows := m.top, m.rowsIn(m.top)-m.row
	for rows < h && v+1 < len(m.vis) {
		v++
		rows += m.rowsIn(v)
	}
	return v
}

// covers reports whether the window shows every row of entry v, or its
// first row when v is taller than the window and at its top.
func (m *Model) covers(v int) bool {
	if v < m.top {
		return false
	}
	if v == m.top {
		return m.row == 0
	}
	h := m.bodyHeight()
	rows := 0
	for i := m.top; i < v; i++ {
		rows += m.rowsIn(i)
		if i == m.top {
			rows -= m.row
		}
		if rows >= h {
			return false
		}
	}
	return rows+m.rowsIn(v) <= h
}

// clamp keeps the window and the cursor within the log after anything that
// moved or resized them.
func (m *Model) clamp() {
	m.cur = max(min(m.cur, len(m.vis)-1), 0)
	lastTop, lastRow := m.last()
	if m.top > lastTop || m.top == lastTop && m.row > lastRow {
		m.top, m.row = lastTop, lastRow
	}
	m.top = max(m.top, 0)
	switch {
	case !m.wrap || len(m.vis) == 0:
		m.row = 0
	default:
		m.row = max(min(m.row, m.rowsIn(m.top)-1), 0)
	}
	if m.wrap {
		m.left = 0
	}
	m.left = max(m.left, 0)
}

// show scrolls the window as little as it takes to show the cursor.
func (m *Model) show() {
	if len(m.vis) == 0 || m.bodyHeight() == 0 || m.covers(m.cur) {
		return
	}
	if m.cur <= m.top {
		m.top, m.row = m.cur, 0
		return
	}
	top, row := m.endAt(m.cur)
	if top > m.cur || top == m.cur && row > 0 {
		// Taller than the window: show its first row.
		top, row = m.cur, 0
	}
	m.top, m.row = top, row
	m.clamp()
}

// jumpTo moves the cursor to entry v. When v is out of view, the window
// scrolls to show it a third of the way down, with what led to it above.
func (m *Model) jumpTo(v int) {
	m.cur = v
	if !m.covers(v) {
		m.top, m.row = max(v-m.bodyHeight()/3, 0), 0
		m.clamp()
		m.show()
	}
}

// move moves the cursor n entries, scrolling as little as it takes.
func (m *Model) move(n int) {
	m.cur += n
	m.clamp()
	m.show()
}

// page scrolls the window n rows, down for a positive n, and moves the
// cursor as many entries, keeping it in the window.
func (m *Model) page(n int) {
	if n > 0 {
		m.down(n)
	} else {
		m.up(-n)
	}
	m.cur += n
	m.clamp()
	m.cur = max(min(m.cur, m.bottom()), m.top)
	m.show()
}

// home moves the cursor and the window to the start.
func (m *Model) home() {
	m.cur, m.top, m.row = 0, 0, 0
}

// end moves the cursor and the window to the end.
func (m *Model) end() {
	m.cur = max(len(m.vis)-1, 0)
	m.top, m.row = m.last()
	m.clamp()
}

// scrollRight scrolls n columns right, but no further than the longest
// line in the window reaches.
func (m *Model) scrollRight(n int) {
	l := m.layout()
	want := m.left + n
	reach := 0
	for v, last := m.top, m.bottom(); v >= 0 && v <= last; v++ {
		r := &m.rows[m.vis[v]]
		if r.fold >= 0 {
			continue
		}
		tw := m.textWidth(l, r)
		// One more column finds a wide rune that crosses the edge.
		_, w := advance(r.text, 0, want+tw+1)
		reach = max(reach, w-tw)
	}
	m.left = max(min(want, reach), m.left)
}

// hStep returns the columns one sideways scroll moves.
func (m *Model) hStep() int { return max((m.width-m.layout().width())/4, 1) }
