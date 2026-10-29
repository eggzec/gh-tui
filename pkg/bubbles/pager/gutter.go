package pager

import (
	"strconv"
	"strings"
)

// Gutter draws the cells before the text of a row and says how many there
// are. The pager's own line numbers are a Gutter.
type Gutter interface {
	// Width returns the cells the gutter takes in a pager of total columns,
	// or 0 to hide it. It may shrink at narrow widths to leave room for
	// the text. A width of total or more leaves no room for text, so the
	// pager hides the gutter.
	Width(total int) int
	// Write writes exactly Width cells for row i. first is true for the
	// first screen row of the row, and false for the rows a wrapped row
	// continues on.
	Write(b *strings.Builder, i int, first bool)
}

// numberGutter is the default gutter: the line numbers and the space after
// them, or the marks of an inverted search while the numbers are hidden.
// w is the width the pager drew it at, which Write fills.
type numberGutter struct {
	m *Model
	w int
}

func (g numberGutter) Width(total int) int {
	n := g.m.rows.Len()
	if n == 0 {
		return 0
	}
	return g.widthFor(n+g.m.reserved(), total)
}

// widthFor returns the width of the gutter of n lines in total columns.
func (g numberGutter) widthFor(n, total int) int {
	var w int
	switch {
	case n == 0:
		return 0
	case g.m.lineNumbers:
		w = len(strconv.Itoa(n)) + 1
	case g.m.search.invert:
		w = 2
	default:
		return 0
	}
	if w >= total {
		return 0
	}
	return w
}

func (g numberGutter) Write(b *strings.Builder, i int, first bool) {
	m := g.m
	if !first {
		b.WriteString(strings.Repeat(" ", g.w))
		return
	}
	st := m.esc.number
	s := m.search
	_, _, hit := m.lineHits(i)
	switch {
	case s.invert && s.cur >= 0 && i == s.curLine:
		st = m.esc.current
	case s.invert && hit:
		st = m.esc.match
	case i == m.mark:
		st = m.esc.current
	}
	if !m.lineNumbers {
		// Only the marks of an inverted search show, in a cell of their
		// own.
		b.WriteString(st.on)
		b.WriteString(" ")
		b.WriteString(st.off)
		b.WriteString(" ")
		return
	}
	n := strconv.Itoa(i + 1)
	b.WriteString(st.on)
	b.WriteString(strings.Repeat(" ", g.w-1-len(n)))
	b.WriteString(n)
	b.WriteString(st.off)
	b.WriteByte(' ')
}
