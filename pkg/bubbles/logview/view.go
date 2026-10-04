package logview

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/errline"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// View renders the rows in the window and the status line, in exactly
// Height lines of Width cells. Only the rows in the window are rendered.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	var b strings.Builder
	// Rows carry escape sequences, so leave room for them.
	b.Grow(m.height * (m.width + 96))
	rows := 0
	switch {
	case m.state == stateReady && len(m.vis) > 0:
		rows = m.writeRows(&b)
	case m.state == stateFailed:
		for _, l := range m.errorLines(m.width, m.bodyHeight()) {
			b.WriteString(m.fit(l, m.width))
			b.WriteByte('\n')
			rows++
		}
	default:
		// A message longer than the width wraps, as far as the height
		// lets it.
		for l := range strings.SplitSeq(ansi.Wrap(m.message(), m.width, ""), "\n") {
			if rows >= m.bodyHeight() {
				break
			}
			b.WriteString(m.fit(l, m.width))
			b.WriteByte('\n')
			rows++
		}
	}
	for ; rows < m.bodyHeight(); rows++ {
		spaces(&b, m.width)
		b.WriteByte('\n')
	}
	b.WriteString(m.statusLine())
	return b.String()
}

// message returns the placeholder shown instead of rows.
func (m *Model) message() string {
	s := m.styles
	switch m.state {
	case stateLoading:
		return m.spin.View() + s.Message.Render("Loading the log"+cmp.Or(s.Ellipsis, "…"))
	case stateReady:
		return s.Message.Render("No output yet.")
	default:
		return s.Message.Render("Nothing to show.")
	}
}

// errorWords returns what the view says of the failed load, and the hint
// after it.
func (m *Model) errorWords() (text, hint string) {
	if m.errorText != nil && m.err != nil {
		return m.errorText(m.err)
	}
	msg := "unknown error"
	if m.err != nil {
		msg, _, _ = strings.Cut(m.err.Error(), "\n")
	}
	return "Couldn't load the log: " + msg, ""
}

// errorLines renders the failed load in at most height lines of width
// cells, as the app's error lines are.
func (m *Model) errorLines(width, height int) []string {
	s := m.styles
	st := errline.Styles{
		Mark: s.ErrorGlyph, Separator: s.ErrorSeparator, Ellipsis: s.ErrorEllipsis,
		Text: s.LoadError, Hint: s.Message,
	}
	return errline.Fit(st, m.errText, m.errHint, width, height)
}

// writeRows writes the rows of the window, each followed by a newline, and
// returns how many it wrote.
func (m *Model) writeRows(b *strings.Builder) int {
	h, l := m.bodyHeight(), m.layout()
	rows := 0
	for v := m.top; v < len(m.vis) && rows < h; v++ {
		ri := m.vis[v]
		r := &m.rows[ri]
		if l.width()+indent(r) >= m.width {
			// Too narrow for any text: cut the gutter.
			var t strings.Builder
			m.writeGutter(&t, l, v, r, true)
			m.writeMarker(&t, r)
			b.WriteString(m.fit(t.String(), m.width))
			b.WriteByte('\n')
			rows++
			continue
		}
		tw := m.textWidth(l, r)
		if r.fold >= 0 {
			m.writeHeader(b, l, v, ri, r, tw)
			b.WriteByte('\n')
			rows++
			continue
		}
		if !m.wrap {
			a, pad := leftEdge(r.text, m.left)
			e, used := advance(r.text, a, tw-pad)
			m.writeGutter(b, l, v, r, true)
			spaces(b, pad)
			m.writeText(b, ri, r, a, e)
			spaces(b, tw-pad-used)
			b.WriteByte('\n')
			rows++
			continue
		}
		for sub, a := 0, 0; rows < h; sub++ {
			e, used := nextRow(r.text, a, tw)
			if v > m.top || sub >= m.row {
				m.writeGutter(b, l, v, r, sub == 0)
				if used > tw {
					// Only a grapheme wider than the whole text column gets
					// here.
					spaces(b, tw)
				} else {
					m.writeText(b, ri, r, a, e)
					spaces(b, tw-used)
				}
				b.WriteByte('\n')
				rows++
			}
			if a = e; a >= len(r.text) {
				break
			}
		}
	}
	return rows
}

// leftEdge returns the first byte of s shown when scrolled left columns
// sideways, and the blank cells before it where a wide grapheme is cut in
// half.
func leftEdge(s string, left int) (a, pad int) {
	a, used := advance(s, 0, left)
	if used < left && a < len(s) {
		e, w := advance(s, a, 2)
		return e, used + w - left
	}
	return a, 0
}

// writeGutter writes the cursor, the line number, the mark, the time and
// the indent of row r, shown by entry v. Only the first row of a wrapped
// line has the number, mark and time.
func (m *Model) writeGutter(b *strings.Builder, l layout, v int, r *row, first bool) {
	switch {
	case v != m.cur:
		b.WriteByte(' ')
	case m.focused:
		b.WriteString(m.esc.cursor)
	default:
		b.WriteString(m.esc.blurred)
	}
	if l.numbers > 0 {
		if first && r.kind != kindSection {
			var buf [20]byte
			n := strconv.AppendInt(buf[:0], int64(r.src+1), 10)
			spaces(b, l.numbers-1-len(n))
			b.WriteString(m.esc.number.on)
			b.Write(n)
			b.WriteString(m.esc.number.off)
			b.WriteByte(' ')
		} else {
			spaces(b, l.numbers)
		}
	}
	mk := ""
	if first {
		mk = m.esc.mark(m, r)
	}
	if mk == "" {
		mk = " "
	}
	b.WriteString(mk)
	b.WriteByte(' ')
	if l.times > 0 {
		if first {
			m.writeTime(b, r)
		} else {
			spaces(b, timeWidth)
		}
		b.WriteByte(' ')
	}
	spaces(b, 2*r.depth)
}

// writeMarker writes the glyph of an open or a folded fold before the row
// that starts it, or blanks when the fold is empty.
func (m *Model) writeMarker(b *strings.Builder, r *row) {
	switch {
	case r.fold < 0:
	case m.folds[r.fold].end == m.folds[r.fold].head+1:
		spaces(b, 2)
	case m.folds[r.fold].open:
		b.WriteString(m.esc.open)
	default:
		b.WriteString(m.esc.closed)
	}
}

// writeTime writes the time of row r in timeWidth cells.
func (m *Model) writeTime(b *strings.Builder, r *row) {
	if r.time.IsZero() {
		spaces(b, timeWidth)
		return
	}
	var buf [24]byte
	var t []byte
	if m.times == TimeAbsolute {
		t = r.time.AppendFormat(buf[:0], "15:04:05")
	} else {
		began := m.began
		if r.sec >= 0 && !m.secs[r.sec].began.IsZero() {
			began = m.secs[r.sec].began
		}
		t = appendSince(buf[:0], r.time.Sub(began))
	}
	t = t[:min(len(t), timeWidth)]
	spaces(b, timeWidth-len(t))
	b.WriteString(m.esc.time.on)
	b.Write(t)
	b.WriteString(m.esc.time.off)
}

// appendSince appends a time since the start of a section: minutes,
// seconds and tenths under an hour, and hours, minutes and seconds after.
func appendSince(buf []byte, d time.Duration) []byte {
	d = max(d, 0)
	buf = append(buf, '+')
	if d < time.Hour {
		buf = appendTwo(buf, int(d/time.Minute))
		buf = append(buf, ':')
		buf = appendTwo(buf, int(d/time.Second%60))
		buf = append(buf, '.')
		return strconv.AppendInt(buf, int64(d/(time.Second/10)%10), 10)
	}
	buf = strconv.AppendInt(buf, int64(d/time.Hour), 10)
	buf = append(buf, ':')
	buf = appendTwo(buf, int(d/time.Minute%60))
	buf = append(buf, ':')
	return appendTwo(buf, int(d/time.Second%60))
}

// appendTwo appends n with at least two digits.
func appendTwo(buf []byte, n int) []byte {
	if n < 10 {
		buf = append(buf, '0')
	}
	return strconv.AppendInt(buf, int64(n), 10)
}

// writeHeader writes the row that starts a fold: its marker and title, and
// how long a section took at the right edge.
func (m *Model) writeHeader(b *strings.Builder, l layout, v, ri int, r *row, tw int) {
	m.writeGutter(b, l, v, r, true)
	m.writeMarker(b, r)
	var buf [16]byte
	var dur []byte
	if r.kind == kindSection && m.secs[r.sec].duration > 0 {
		dur = appendDuration(buf[:0], m.secs[r.sec].duration)
	}
	room := tw
	if len(dur) > 0 && len(dur)+minText/2 < tw {
		room = tw - len(dur) - 1
	} else {
		dur = nil
	}
	e, used := advance(r.text, 0, room)
	cut := e < len(r.text)
	ell, ew := m.esc.ellipsis, m.esc.ellipsisWidth
	if ew > room {
		ell, ew = "", 0
	}
	if cut {
		e, used = advance(r.text, 0, room-ew)
	}
	m.writeText(b, ri, r, 0, e)
	if cut && ell != "" {
		base := m.esc.text(m, r)
		b.WriteString(base.on)
		b.WriteString(ell)
		b.WriteString(base.off)
		used += ew
	}
	spaces(b, room-used)
	if len(dur) > 0 {
		b.WriteByte(' ')
		b.WriteString(m.esc.duration.on)
		b.Write(dur)
		b.WriteString(m.esc.duration.off)
	}
}

// appendDuration appends how long a step took, the way GitHub shows it:
// "0s", "42s", "1m 5s" or "1h 2m".
func appendDuration(buf []byte, d time.Duration) []byte {
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return append(strconv.AppendInt(buf, int64(d/time.Second), 10), 's')
	case d < time.Hour:
		buf = append(strconv.AppendInt(buf, int64(d/time.Minute), 10), "m "...)
		return append(strconv.AppendInt(buf, int64(d/time.Second%60), 10), 's')
	default:
		buf = append(strconv.AppendInt(buf, int64(d/time.Hour), 10), "h "...)
		return append(strconv.AppendInt(buf, int64(d/time.Minute%60), 10), 'm')
	}
}

// writeText writes bytes a to e of the text of row r, which is row ri, in
// the style of its kind under its own colors, or in the style of the
// matches over them.
func (m *Model) writeText(b *strings.Builder, ri int, r *row, a, e int) {
	base := m.esc.text(m, r)
	matches, first := m.rowMatches(ri)
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
		for mi < len(matches) && matches[mi].end <= pos {
			mi++
		}
		next := e
		if k < len(marks) {
			next = min(next, marks[k].Pos)
		}
		if mi < len(matches) {
			x := matches[mi]
			if x.start > pos {
				next = min(next, x.start)
			} else {
				if !inMatch {
					p := m.esc.match
					if first+mi == m.search.cur {
						p = m.esc.current
					}
					b.WriteString(ansi.ResetStyle)
					b.WriteString(p.on)
					pen.Lost()
					inMatch = true
				}
				next = min(next, x.end)
			}
		}
		b.WriteString(r.text[pos:next])
		pos = next
		if inMatch && pos >= matches[mi].end {
			pen.Write(b, cur)
			inMatch = false
		}
	}
	b.WriteString(ansi.ResetStyle)
}

// statusLine renders the title on the left and where the cursor is on the
// right, or the search input while it is open.
func (m *Model) statusLine() string {
	if m.searching {
		return m.fit(m.input.View(), m.width)
	}
	var parts []string
	if q := m.search.query; q != "" {
		if n := len(m.search.matches); n == 0 {
			parts = append(parts, m.esc.noMatches.wrap("no matches"))
		} else {
			parts = append(parts, m.esc.status.wrap(fmt.Sprintf("match %d/%d", m.search.cur+1, n)))
		}
	}
	switch {
	case m.jumped == Warning:
		parts = append(parts, m.esc.status.wrap(fmt.Sprintf("warning %d/%d", m.at+1, len(m.warns))))
	case m.jumped == Error:
		parts = append(parts, m.esc.status.wrap(fmt.Sprintf("error %d/%d", m.at+1, len(m.errs))))
	case len(m.errs) == 1:
		parts = append(parts, m.esc.status.wrap("1 error"))
	case len(m.errs) > 1:
		parts = append(parts, m.esc.status.wrap(fmt.Sprintf("%d errors", len(m.errs))))
	}
	if m.live && m.follow {
		parts = append(parts, m.esc.status.wrap("follow"))
	}
	if m.state == stateReady && len(m.vis) > 0 && m.bodyHeight() > 0 {
		parts = append(parts, m.esc.status.wrap(m.position()))
	}
	right := strings.Join(parts, "  ")
	rw := ansi.StringWidth(right)
	if rw+2 > m.width {
		return m.fit(right, m.width)
	}
	return m.fit(m.titleView, m.width-rw-2) + "  " + right
}

// position is where the cursor is. With nothing folded it is the line of
// the log, as the gutter numbers it, and the percent counts lines. While
// folds hide rows, it is the row among those shown, and the percent
// counts rows, so a log folded to its step titles doesn't read as a few
// lines of hundreds.
func (m *Model) position() string {
	if len(m.vis) < len(m.rows) || m.n == 0 {
		pct := (m.bottom() + 1) * 100 / len(m.vis)
		return fmt.Sprintf("row %d/%d  %d%%", m.cur+1, len(m.vis), pct)
	}
	line := min(m.rows[m.vis[m.cur]].src+1, m.n)
	pct := min(m.rows[m.vis[m.bottom()]].src+1, m.n) * 100 / m.n
	return fmt.Sprintf("line %d/%d  %d%%", line, m.n, pct)
}

// spaces writes n spaces.
func spaces(b *strings.Builder, n int) {
	const blank = "                                                                "
	for n > len(blank) {
		b.WriteString(blank)
		n -= len(blank)
	}
	if n > 0 {
		b.WriteString(blank[:n])
	}
}

// fit truncates styled text to exactly width cells, ending it with the
// ellipsis where it is cut, or pads it.
func (m *Model) fit(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		tail := m.esc.ellipsis
		if m.esc.ellipsisWidth > width {
			tail = ""
		}
		s = ansi.Truncate(s, width, tail)
		w = ansi.StringWidth(s)
	}
	if w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}
