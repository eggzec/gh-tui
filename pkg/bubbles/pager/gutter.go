package pager

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
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

// WithGutter draws the cells before each row with g in place of the line
// numbers. The numbers stay the default, and a nil g restores them.
func WithGutter(g Gutter) Option {
	return func(s *settings) { s.gutter = g }
}

// Pair is the escape sequences that turn a style on and off, made once from
// a style so that drawing a row costs no rendering. The zero Pair has no
// style.
type Pair struct{ p pair }

// NewPair returns the Pair of a style.
func NewPair(s lipgloss.Style) Pair { return Pair{newPair(s)} }

// RowStyle gives the base style of row i: the style of its text where no
// token or match colors it. The pager asks it for each row it draws, so it
// must be cheap and must not allocate. A zero Pair keeps the text style.
type RowStyle func(i int) Pair

// WithRowStyle styles the text of each row with the style that f gives it.
func WithRowStyle(f RowStyle) Option {
	return func(s *settings) { s.rowStyle = f }
}

// Headed is an optional interface of [Rows]: a row that reports a header is
// drawn by the [HeaderWriter] as one row, never wrapped.
type Headed interface {
	// Header returns the text of header row i, and false for the rows that
	// are not headers.
	Header(i int) (text string, ok bool)
}

// Indenter is an optional interface of [Rows]: row i is drawn Indent(i)
// blank cells right of the gutter, on each of its screen rows, and wraps in
// the columns that are left. Header rows are drawn whole and ignore it.
type Indenter interface {
	// Indent returns the blank cells before row i. It is asked while the
	// pager draws and measures, so it must be cheap.
	Indent(i int) int
}

// HeaderWriter draws header row i, whose text is given, over the whole width
// of the pager: it writes exactly width cells, the gutter's included.
type HeaderWriter func(b *strings.Builder, i int, text string, width int)

// WithHeaderWriter draws the rows that a [Headed] source reports as headers
// with w. Without it every row is drawn as the others are.
func WithHeaderWriter(w HeaderWriter) Option {
	return func(s *settings) { s.headerWriter = w }
}

// header returns the text of row i when it is a header row.
func (m *Model) header(i int) (string, bool) {
	if m.headerWriter == nil {
		return "", false
	}
	h, ok := m.rows.(Headed)
	if !ok {
		return "", false
	}
	return h.Header(i)
}

// base returns the style of the text of row i.
func (m Model) base(i int) pair {
	if m.rowStyle == nil {
		return m.esc.text
	}
	if p := m.rowStyle(i).p; p.on != "" || p.off != "" {
		return p
	}
	return m.esc.text
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
	case m.cursoring && i == m.cursor, i == m.mark:
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
