package pager

import (
	"strconv"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// advance walks the graphemes of the plain text s from byte i, as long as
// they fit in cols cells. It returns the byte where it stopped and the
// cells it walked. Unlike ansi.Truncate it stops there, so walking a window
// of a long line costs the window, not the line.
func advance(s string, i, cols int) (j, used int) {
	for j = i; j < len(s); {
		// ASCII followed by ASCII is a grapheme of its own; clean has
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

// bodyHeight returns the rows above the status line.
func (m Model) bodyHeight() int { return max(m.height-1, 0) }

// gutterWidth returns the width of the line numbers and the space after
// them, or of the marks of an inverted search while the numbers are
// hidden, or 0 when there is neither or no room for text.
func (m Model) gutterWidth() int {
	var w int
	switch {
	case len(m.lines) == 0:
		return 0
	case m.lineNumbers:
		w = len(strconv.Itoa(len(m.lines))) + 1
	case m.search.invert:
		w = 2
	default:
		return 0
	}
	if w >= m.width {
		return 0
	}
	return w
}

// textWidth returns the columns right of the gutter.
func (m Model) textWidth() int { return max(m.width-m.gutterWidth(), 0) }

// rowsIn returns how many rows the line shown at p takes.
func (m Model) rowsIn(p int) int {
	if !m.wrap {
		return 1
	}
	s, tw := m.lines[m.at(p)], m.textWidth()
	n := 1
	for j, _ := nextRow(s, 0, tw); j < len(s); j, _ = nextRow(s, j, tw) {
		n++
	}
	return n
}

// rowOf returns the row of the line shown at p that holds byte b.
func (m Model) rowOf(p, b int) int {
	s, tw := m.lines[m.at(p)], m.textWidth()
	r := 0
	for j, _ := nextRow(s, 0, tw); j <= b && j < len(s); j, _ = nextRow(s, j, tw) {
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
		return min(m.top+h, n) - 1
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
	if len(m.lines) == 0 || n < 1 {
		return
	}
	i := min(n, len(m.lines)) - 1
	m.mark = i
	m.top, m.row = max(m.posOf(i)-m.bodyHeight()/3, 0), 0
	m.clamp()
}

// goTo scrolls line n, counted from 1, to the top of the window, or the
// first line shown after it, as less's g does with a count. A line past
// the end goes to the last.
func (m *Model) goTo(n int) {
	if m.count() == 0 {
		return
	}
	i := max(min(n, len(m.lines)), 1) - 1
	m.top, m.row = min(m.posOf(i), m.count()-1), 0
	m.clamp()
}

// goToPercent scrolls the line n percent of the way into the content to
// the top of the window, as less's % does: 0 goes to the first line and
// 100 or more to the last. It counts the lines of the content, as the
// percent on the status line does, so a filter goes to the first line
// shown from there.
func (m *Model) goToPercent(n int) {
	m.goTo(min(n, 100) * len(m.lines) / 100)
}

// clamp keeps the window within the content after anything that moved or
// resized it.
func (m *Model) clamp() {
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
	tw := m.textWidth()
	want := m.left + n
	reach := 0
	for p, last := m.top, m.bottom(); p <= last; p++ {
		// One more column finds a wide rune that crosses the edge.
		_, w := advance(m.lines[m.at(p)], 0, want+tw+1)
		reach = max(reach, w)
	}
	m.left = max(min(want, reach-tw), m.left)
}

// hStep returns the columns one sideways scroll moves.
func (m Model) hStep() int { return max(m.textWidth()/4, 1) }
