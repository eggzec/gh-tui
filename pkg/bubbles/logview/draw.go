package logview

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/internal/wrap"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The pager draws and scrolls the log. It reads the rows from the rowStore,
// which also answers its hooks: the gutter, the fold headers, the indent of
// each row and its base style. The hooks are set once, in New, while the
// model is a value that is copied, so they read a copy of what they need,
// the look, which the model refreshes on the shared store before the pager
// measures or draws.

// look is what the pager's hooks read of the model.
type look struct {
	width       int
	focused     bool
	lineNumbers bool
	times       TimeMode
	n           int
	folds       []fold
	secs        []section
	began       time.Time
	esc         esc
	cursor      int
	matching    bool
	// ranges returns the matches in row i and which of them is current,
	// or -1. View sets it through the shared store.
	ranges func(i int) ([][]int, int)
}

// syncLook copies what the pager's hooks read onto the shared store.
func (m *Model) syncLook() {
	// A fold the pager revealed lives only on the store until adopted; copying
	// the model's folds over it would undo the reveal.
	m.adoptReveal()
	s := m.store
	s.rows = m.rows
	s.look = look{
		width:       m.width,
		focused:     m.focused,
		lineNumbers: m.lineNumbers,
		times:       m.times,
		n:           m.n,
		folds:       m.folds,
		secs:        m.secs,
		began:       m.began,
		esc:         m.esc,
		cursor:      m.cursorRow(),
		matching:    m.matching,
	}
}

// noMarks is the marks of a row with no colors of its own: the rows of a
// log are drawn through their colors, an empty row too.
var noMarks = []termtext.Style{}

// Marks returns the colors of row i, never nil, so that the pager draws
// every row through them.
func (s *rowStore) Marks(i int) []termtext.Style {
	if i < 0 || i >= len(s.rows) || s.rows[i].marks == nil {
		return noMarks
	}
	return s.rows[i].marks
}

// Header reports the rows that start a fold, which the header writer draws.
func (s *rowStore) Header(i int) (string, bool) {
	if i < 0 || i >= len(s.rows) || s.rows[i].fold < 0 {
		return "", false
	}
	return s.rows[i].text, true
}

// Indent returns the cells before the text of row i.
func (s *rowStore) Indent(i int) int {
	if i < 0 || i >= len(s.rows) {
		return 0
	}
	return indent(&s.rows[i])
}

// rowStyle returns the base style of row i: of its kind, or of its section.
func (s *rowStore) rowStyle(i int) pager.Pair {
	if i < 0 || i >= len(s.rows) {
		return pager.Pair{}
	}
	r, e := &s.rows[i], &s.look.esc
	switch {
	case r.kind == kindSection && s.look.secs[r.sec].failed:
		return e.failedRow
	case r.kind == kindSection:
		return e.sectionRow
	case int(r.kind) < len(e.kindRows):
		return e.kindRows[r.kind]
	default:
		return e.kindRows[Plain]
	}
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

// width returns the width of the gutter: the cursor, the numbers, the mark
// and a space, and the times.
func (l layout) width() int { return 1 + l.numbers + 2 + l.times }

// digits returns the number of decimal digits of n > 0.
func digits(n int) int {
	d := 1
	for ; n >= 10; n /= 10 {
		d++
	}
	return d
}

// indent returns the cells before the text of row r: two for each fold it
// is in, and two for the marker of a fold it starts.
func indent(r *row) int {
	n := 2 * r.depth
	if r.fold >= 0 {
		n += 2
	}
	return n
}

// layout returns the widths of the parts of the gutter in width columns.
func (l look) layout(width int) layout {
	return layoutFor(l.lineNumbers, l.n, l.times, width)
}

// layoutFor returns the widths of the parts of the gutter in width columns:
// the times and then the line numbers go when they leave too little text.
func layoutFor(lineNumbers bool, n int, times TimeMode, width int) layout {
	var l layout
	if lineNumbers && n > 0 {
		l.numbers = digits(n) + 1
	}
	if times != TimeHidden {
		l.times = timeWidth + 1
	}
	if l.width()+minText > width {
		l.times = 0
	}
	if l.width()+minText > width {
		l.numbers = 0
	}
	return l
}

// gutter draws the cursor, the line number, the mark of errors and warnings,
// and the time before each row.
type gutter struct{ s *rowStore }

func (g gutter) Width(total int) int { return g.s.look.layout(total).width() }

func (g gutter) Write(b *strings.Builder, i int, first bool) {
	g.WriteCursor(b, i, first, false)
}

func (g gutter) WriteCursor(b *strings.Builder, i int, first, cursor bool) {
	s := g.s
	s.writeGutter(b, s.look.layout(s.look.width), &s.rows[i], first, cursor)
}

// writeGutter writes the gutter of row r. Only the first row of a wrapped
// line has the number, mark and time.
func (s *rowStore) writeGutter(b *strings.Builder, l layout, r *row, first, cursor bool) {
	lk := &s.look
	switch {
	case !cursor:
		b.WriteByte(' ')
	case lk.focused:
		b.WriteString(lk.esc.cursor)
	default:
		b.WriteString(lk.esc.blurred)
	}
	if l.numbers > 0 {
		if first && r.kind != kindSection {
			var buf [20]byte
			n := strconv.AppendInt(buf[:0], int64(r.src+1), 10)
			spaces(b, l.numbers-1-len(n))
			b.WriteString(lk.esc.number.on)
			b.Write(n)
			b.WriteString(lk.esc.number.off)
			b.WriteByte(' ')
		} else {
			spaces(b, l.numbers)
		}
	}
	mk := ""
	if first {
		mk = lk.mark(r)
	}
	if mk == "" {
		mk = " "
	}
	b.WriteString(mk)
	b.WriteByte(' ')
	if l.times > 0 {
		if first {
			s.writeTime(b, r)
		} else {
			spaces(b, timeWidth)
		}
		b.WriteByte(' ')
	}
}

// mark returns the mark in the gutter of row r, in its style, or "".
func (l *look) mark(r *row) string {
	switch {
	case r.kind == Error, r.kind == kindSection && l.secs[r.sec].failed:
		return l.esc.errorMark
	case r.kind == Warning:
		return l.esc.warningMark
	case r.kind == Notice:
		return l.esc.noticeMark
	default:
		return ""
	}
}

// text returns the base style of row r: of its kind, or of its section.
func (l *look) text(r *row) pair {
	e := &l.esc
	switch {
	case r.kind == kindSection && l.secs[r.sec].failed:
		return e.failed
	case r.kind == kindSection:
		return e.section
	case int(r.kind) < len(e.kinds):
		return e.kinds[r.kind]
	default:
		return e.kinds[Plain]
	}
}

// writeTime writes the time of row r in timeWidth cells.
func (s *rowStore) writeTime(b *strings.Builder, r *row) {
	lk := &s.look
	if r.time.IsZero() {
		spaces(b, timeWidth)
		return
	}
	var buf [24]byte
	var t []byte
	if lk.times == TimeAbsolute {
		t = r.time.AppendFormat(buf[:0], "15:04:05")
	} else {
		began := lk.began
		if r.sec >= 0 && !lk.secs[r.sec].began.IsZero() {
			began = lk.secs[r.sec].began
		}
		t = appendSince(buf[:0], r.time.Sub(began))
	}
	t = t[:min(len(t), timeWidth)]
	spaces(b, timeWidth-len(t))
	b.WriteString(lk.esc.time.on)
	b.Write(t)
	b.WriteString(lk.esc.time.off)
}

// writeMarker writes the glyph of an open or a folded fold before the row
// that starts it, or blanks when the fold is empty.
func (s *rowStore) writeMarker(b *strings.Builder, r *row) {
	lk := &s.look
	switch {
	case r.fold < 0:
	case lk.folds[r.fold].end == lk.folds[r.fold].head+1:
		spaces(b, 2)
	case lk.folds[r.fold].open:
		b.WriteString(lk.esc.open)
	default:
		b.WriteString(lk.esc.closed)
	}
}

// writeHeader writes row i, which starts a fold, over width cells: its
// gutter, marker and title, and how long a section took at the right edge.
func (s *rowStore) writeHeader(b *strings.Builder, i int, _ string, width int) {
	lk := &s.look
	r := &s.rows[i]
	l := lk.layout(width)
	cursor := i == lk.cursor
	if l.width()+indent(r) >= width {
		// Too narrow for any text: cut the gutter.
		var t strings.Builder
		s.writeGutter(&t, l, r, true, cursor)
		spaces(&t, 2*r.depth)
		s.writeMarker(&t, r)
		b.WriteString(fitTo(t.String(), width, lk.esc.ellipsis, lk.esc.ellipsisWidth))
		return
	}
	tw := width - l.width() - indent(r)
	s.writeGutter(b, l, r, true, cursor)
	spaces(b, 2*r.depth)
	s.writeMarker(b, r)
	var buf [16]byte
	var dur []byte
	if r.kind == kindSection && lk.secs[r.sec].duration > 0 {
		dur = appendDuration(buf[:0], lk.secs[r.sec].duration)
	}
	room := tw
	if len(dur) > 0 && len(dur)+minText/2 < tw {
		room = tw - len(dur) - 1
	} else {
		dur = nil
	}
	e, used := wrap.Advance(r.text, 0, room)
	cut := e < len(r.text)
	ell, ew := lk.esc.ellipsis, lk.esc.ellipsisWidth
	if ew > room {
		ell, ew = "", 0
	}
	if cut {
		e, used = wrap.Advance(r.text, 0, room-ew)
	}
	s.writeText(b, i, 0, e)
	if cut && ell != "" {
		base := lk.text(r)
		b.WriteString(base.on)
		b.WriteString(ell)
		b.WriteString(base.off)
		used += ew
	}
	spaces(b, room-used)
	if len(dur) > 0 {
		b.WriteByte(' ')
		b.WriteString(lk.esc.duration.on)
		b.Write(dur)
		b.WriteString(lk.esc.duration.off)
	}
}

// writeText writes bytes a to e of the text of row i in the style of its
// kind under its own colors, or in the style of the matches over them.
func (s *rowStore) writeText(b *strings.Builder, i, a, e int) {
	lk := &s.look
	r := &s.rows[i]
	base := lk.text(r)
	var matches [][]int
	curMatch := -1
	if lk.matching && lk.ranges != nil {
		matches, curMatch = lk.ranges(i)
	}
	marks := r.marks
	if base.on == "" && len(marks) == 0 && len(matches) == 0 {
		b.WriteString(r.text[a:e])
		return
	}
	// k counts the marks before or at a, the last of which is the style
	// there.
	k, _ := slices.BinarySearchFunc(marks, a+1, func(x termtext.Style, pos int) int { return x.Pos - pos })
	cur := ""
	if k > 0 {
		cur = marks[k-1].Seq
	}
	// The pen writes only what changes from one style to the next.
	pen := base.pen
	pen.Start(b)
	pen.Write(b, cur)
	mi, inMatch := 0, false
	for pos := a; pos < e; {
		if k < len(marks) && marks[k].Pos <= pos {
			for k < len(marks) && marks[k].Pos <= pos {
				k++
			}
			cur = marks[k-1].Seq
			// A match hides the log's colors, which come back after it.
			if !inMatch {
				pen.Write(b, cur)
			}
		}
		for mi < len(matches) && matches[mi][1] <= pos {
			mi++
		}
		next := e
		if k < len(marks) {
			next = min(next, marks[k].Pos)
		}
		if mi < len(matches) {
			x := matches[mi]
			if x[0] > pos {
				next = min(next, x[0])
			} else {
				if !inMatch {
					p := lk.esc.match
					if mi == curMatch {
						p = lk.esc.current
					}
					b.WriteString(ansi.ResetStyle)
					b.WriteString(p.on)
					pen.Lost()
					inMatch = true
				}
				next = min(next, x[1])
			}
		}
		b.WriteString(r.text[pos:next])
		pos = next
		if inMatch && pos >= matches[mi][1] {
			pen.Write(b, cur)
			inMatch = false
		}
	}
	b.WriteString(ansi.ResetStyle)
}

// fitTo truncates styled text to exactly width cells, ending it with ell,
// ellw cells wide, where it is cut, or pads it.
func fitTo(s string, width int, ell string, ellw int) string {
	w := ansi.StringWidth(s)
	if w > width {
		if ellw > width {
			ell = ""
		}
		s = ansi.Truncate(s, width, ell)
		w = ansi.StringWidth(s)
	}
	if w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}
