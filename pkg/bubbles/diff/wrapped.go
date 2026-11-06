package diff

import (
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/internal/wrap"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// With wrapping on, a line that does not fit continues on the rows below it.
// The layout keeps one row per line, so the window is a line, top, and the
// first visual row of it that is shown, sub, as the pager's is. Everything
// else, such as the cursor and the anchors, stays on lines.

// segment is the part of a wrapped line a visual row shows: the bytes [a, e) of
// its cleaned text. The zero value is a line shown whole.
type segment struct {
	on   bool
	a, e int
}

// wrapped reports whether row is drawn over as many visual rows as it needs:
// the lines that scroll sideways, but not the rows of blocks, which are
// already cut to the width.
func (m Model) wrapped(row Row) bool {
	return m.wrapOn && scrolls(row.Kind) && row.Kind != KindBlock
}

// blockRows is the number of visual rows of a row of a block. Prose fits the
// width and takes one; a line of a code fence, which a block may leave wider,
// continues on the rows below it, cut every blockWidth cells.
func (m Model) blockRows(row Row) int {
	w := m.blockWidth()
	if !m.wrapOn || w <= 0 {
		return 1
	}
	return max((ansi.StringWidth(row.Text)+w-1)/w, 1)
}

// textCols is the width of the text of a line in a row, after its marker: a
// raw line has no marker, so its text has the blank the others give to it.
func (m Model) textCols(row Row, numW int) int {
	if row.Kind == KindRaw {
		return m.textWidth(numW) + 1
	}
	return m.textWidth(numW)
}

// rowsIn is the number of visual rows of line r, in a view whose numbers are
// numW wide. The caller reads [Model.numWidth] once and passes it on, since it
// walks the files in the window.
func (m Model) rowsIn(r, numW int) int {
	if !m.wrapOn {
		return 1
	}
	row, ok := m.layout.RowAt(r)
	if ok && row.Kind == KindBlock {
		return m.blockRows(row)
	}
	if !ok || !m.wrapped(row) {
		return 1
	}
	return wrap.Rows(termtext.Clean(row.Text, m.tabs), m.textCols(row, numW))
}

// segmentOf is the part of row that its visual row sub shows, in a view whose
// numbers are numW wide: the same walk that counts the rows.
func (m Model) segmentOf(row Row, numW, sub int) segment {
	if !m.wrapped(row) {
		return segment{}
	}
	a, e, _ := wrap.Segment(termtext.Clean(row.Text, m.tabs), sub, m.textCols(row, numW))
	return segment{on: true, a: a, e: e}
}

// back returns the window that starts k visual rows above the one at (top, sub).
func (m Model) back(top, sub, k, numW int) (newTop, newSub int) {
	for ; k > 0; k-- {
		switch {
		case sub > 0:
			sub--
		case top > 0:
			top--
			sub = m.rowsIn(top, numW) - 1
		default:
			return top, sub
		}
	}
	return top, sub
}

// reaches reports whether the last visual row of line r is among the first h
// rows of the window.
func (m Model) reaches(r, h, numW int) bool {
	if r < m.top {
		return false
	}
	used := -m.sub
	for i := m.top; i <= r; i++ {
		if used += m.rowsIn(i, numW); used > h {
			return false
		}
	}
	return true
}

// inView reports whether all of line r is in the first h rows of the window.
func (m Model) inView(r, h int) bool {
	if !m.wrapOn {
		return r >= m.top && r < m.top+h
	}
	return (r > m.top || r == m.top && m.sub == 0) && m.reaches(r, h, m.numWidth())
}

// centre puts line row in the middle of the window if it is not in it.
func (m *Model) centre(row int) {
	h := m.bodyHeight()
	if m.inView(row, h) {
		return
	}
	if !m.wrapOn {
		m.top = row - h/2
		return
	}
	m.top, m.sub = m.back(row, 0, h/2, m.numWidth())
}

// cursorBy is the line k visual rows from the cursor, down for a positive k.
func (m Model) cursorBy(k int) int {
	if !m.wrapOn {
		return m.cursor + k
	}
	r, n := m.cursor, m.layout.Len()
	off, numW := 0, m.numWidth()
	if k >= 0 {
		for ; r < n-1; r++ {
			rows := m.rowsIn(r, numW)
			if off+rows > k {
				return r
			}
			off += rows
		}
		return r
	}
	for ; r > 0; r-- {
		off += m.rowsIn(r-1, numW)
		if off >= -k {
			return r - 1
		}
	}
	return 0
}

// wrapScroll is [Model.scroll] for a wrapped view: it keeps all the rows of the
// cursor's line in the window, or, for a line taller than the window, the first
// ones.
func (m *Model) wrapScroll(h, n, total int) {
	if total == 0 {
		m.top, m.sub = 0, 0
		return
	}
	numW := m.numWidth()
	m.top = max(min(m.top, total-1), 0)
	m.sub = max(min(m.sub, m.rowsIn(m.top, numW)-1), 0)
	if lt, ls := m.back(total-1, m.rowsIn(total-1, numW)-1, h-1, numW); m.top > lt || m.top == lt && m.sub > ls {
		m.top, m.sub = lt, ls
	}
	// On the last line, the loading or error row shows below it too.
	extra := 0
	if h >= 2 && m.cursor == n-1 && total > n {
		extra = 1
	}
	switch rows := m.rowsIn(m.cursor, numW); {
	case m.cursor < m.top || m.cursor == m.top && m.sub > 0:
		m.top, m.sub = m.cursor, 0
	case rows+extra >= h:
		m.top, m.sub = m.cursor, 0
	case !m.reaches(m.cursor, h-extra, numW):
		m.top, m.sub = m.back(m.cursor, rows-1, h-1-extra, numW)
	}
	// The sticky header covers the window's first row, so the cursor's line must
	// not start there: it starts on the row below, which the line above gives up
	// its last row for. A window that starts above the line already has it there.
	if m.cursor == m.top && m.top > 0 && h >= 3 && !m.isHeader(m.top) {
		if _, ok := m.layout.FileAt(m.top); ok {
			m.top--
			// The line numbers' width depends on the files in the window, so it is
			// read again for the top it has now.
			m.sub = m.rowsIn(m.top, m.numWidth()) - 1
		}
	}
}

// SetWrap sets whether a line that does not fit continues on the rows below it,
// instead of being cut at the edge and scrolled sideways.
func (m *Model) SetWrap(on bool) {
	if m.wrapOn == on {
		return
	}
	m.wrapOn = on
	m.left, m.sub = 0, 0
	m.syncKeys()
	m.scroll()
}

// Wrap reports whether long lines wrap.
func (m Model) Wrap() bool { return m.wrapOn }
