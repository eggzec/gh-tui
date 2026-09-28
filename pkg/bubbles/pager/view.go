package pager

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// View renders the lines in the window and the status line, in exactly
// Height lines of Width cells. Only the lines in the window are rendered.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	var b strings.Builder
	// Tokens carry escape sequences, so leave room for them.
	b.Grow(m.height * (m.width + 64))
	rows := 0
	switch {
	case m.state == stateReady && len(m.lines) > 0:
		rows = m.writeLines(&b)
	case m.state == stateFailed:
		for _, l := range m.errorLines(m.width, m.bodyHeight()) {
			b.WriteString(fit(l, m.width))
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
			b.WriteString(fit(l, m.width))
			b.WriteByte('\n')
			rows++
		}
	}
	blank := strings.Repeat(" ", m.width)
	for ; rows < m.bodyHeight(); rows++ {
		b.WriteString(blank)
		b.WriteByte('\n')
	}
	b.WriteString(m.statusLine())
	return b.String()
}

// message returns the placeholder shown instead of lines.
func (m Model) message() string {
	s := m.styles
	switch m.state {
	case stateLoading:
		return m.spin.View() + s.Message.Render("Loading…")
	case stateBinary:
		return s.Message.Render("Binary file, not shown.")
	case stateMessage:
		return s.Message.Render(m.note)
	case stateReady:
		return s.Message.Render("Empty file.")
	default:
		return s.Message.Render("Nothing to show.")
	}
}

// errorWords returns what the pager says of the failed load, and the hint
// after it.
func (m *Model) errorWords() (text, hint string) {
	if m.errorText != nil && m.err != nil {
		return m.errorText(m.err)
	}
	msg := "unknown error"
	if m.err != nil {
		msg, _, _ = strings.Cut(m.err.Error(), "\n")
	}
	return "Couldn't load: " + msg, ""
}

// errorLines renders the failed load in at most height lines of width
// cells, as the app's error lines are: the mark and the text, wrapped and
// ending in "…" where it needs more lines than the hint leaves it, then
// " · " and the hint, after the text where it fits and else on a line of
// its own. The hint is never cut, unless the width can't hold it at all.
func (m *Model) errorLines(width, height int) []string {
	text, hint := m.errText, m.errHint
	if text == "" || width <= 0 || height <= 0 {
		return nil
	}
	s := m.styles
	lead := errorGlyph + " "
	indent := strings.Repeat(" ", ansi.StringWidth(lead))
	inner := width - len(indent)
	if inner < 1 {
		lead, indent, inner = "", "", width
	}
	tail := ""
	if hint != "" {
		tail = " · " + hint
	}
	if height == 1 && tail != "" {
		// One line holds the hint first, and what is left of the text.
		room := width - ansi.StringWidth(tail)
		if room < ansi.StringWidth(lead)+1 {
			return []string{s.Message.Render(hint)}
		}
		t := lead + text
		if ansi.StringWidth(t) > room {
			t = ansi.Truncate(t, room, ellipsisGlyph)
		}
		return []string{s.Error.Render(t) + s.Message.Render(tail)}
	}
	most := height
	if tail != "" {
		most = height - 1
	}
	rows := wrapWords(text, inner)
	if len(rows) == 0 {
		rows = []string{""}
	}
	if len(rows) > most {
		cut := ansi.Truncate(rows[most-1]+" "+rows[most], inner-1, "")
		rows = append(rows[:most-1], strings.TrimRight(cut, " ")+ellipsisGlyph)
	}
	lines := make([]string, 0, len(rows)+1)
	for i, r := range rows {
		pre := indent
		if i == 0 {
			pre = lead
		}
		lines = append(lines, s.Error.Render(pre+r))
	}
	if tail == "" {
		return lines
	}
	if n := len(rows); ansi.StringWidth(rows[n-1])+ansi.StringWidth(tail) <= inner {
		lines[n-1] += s.Message.Render(tail)
		return lines
	}
	if ansi.StringWidth(indent+hint) > width {
		indent = ""
	}
	return append(lines, indent+s.Message.Render(hint))
}

// wrapWords wraps s to rows of width cells, between words, and cuts a
// word only when it alone is wider.
func wrapWords(s string, width int) []string {
	var rows []string
	row := ""
	for w := range strings.FieldsSeq(s) {
		for ansi.StringWidth(w) > width {
			if row != "" {
				rows, row = append(rows, row), ""
			}
			head := ansi.Truncate(w, width, "")
			if head == "" {
				// A character wider than the row can't show.
				_, n := utf8.DecodeRuneInString(w)
				head = ellipsisGlyph
				w = w[n:]
			} else {
				w = w[len(head):]
			}
			rows = append(rows, head)
		}
		switch {
		case w == "":
		case row == "":
			row = w
		case ansi.StringWidth(row)+1+ansi.StringWidth(w) <= width:
			row += " " + w
		default:
			rows, row = append(rows, row), w
		}
	}
	if row != "" {
		rows = append(rows, row)
	}
	return rows
}

const (
	// errorGlyph starts the text of a failed load.
	errorGlyph = "✗"
	// ellipsisGlyph ends text that was cut.
	ellipsisGlyph = "…"
)

// writeLines writes the rows of the window, each followed by a newline, and
// returns how many it wrote.
func (m Model) writeLines(b *strings.Builder) int {
	h, gw, tw := m.bodyHeight(), m.gutterWidth(), m.textWidth()
	rows := 0
	for i := m.top; i < len(m.lines) && rows < h; i++ {
		s := m.lines[i]
		if !m.wrap {
			a, pad := m.leftEdge(s)
			e, used := advance(s, a, tw-pad)
			m.writeGutter(b, i, true, gw)
			b.WriteString(strings.Repeat(" ", pad))
			m.writeSpan(b, i, a, e)
			b.WriteString(strings.Repeat(" ", tw-pad-used))
			b.WriteByte('\n')
			rows++
			continue
		}
		for r, a := 0, 0; rows < h; r++ {
			e, used := nextRow(s, a, tw)
			if i > m.top || r >= m.row {
				m.writeGutter(b, i, r == 0, gw)
				if used > tw {
					// Only a grapheme wider than the whole text column
					// gets here.
					b.WriteString(strings.Repeat(" ", tw))
				} else {
					m.writeSpan(b, i, a, e)
					b.WriteString(strings.Repeat(" ", tw-used))
				}
				b.WriteByte('\n')
				rows++
			}
			if a = e; a >= len(s) {
				break
			}
		}
	}
	return rows
}

// leftEdge returns the first byte of s shown when scrolled sideways, and
// the blank cells before it where a wide grapheme is cut in half.
func (m Model) leftEdge(s string) (a, pad int) {
	a, used := advance(s, 0, m.left)
	if used < m.left && a < len(s) {
		e, w := advance(s, a, 2)
		return e, used + w - m.left
	}
	return a, 0
}

func (m Model) writeGutter(b *strings.Builder, i int, first bool, gw int) {
	if gw == 0 {
		return
	}
	if !first {
		b.WriteString(strings.Repeat(" ", gw))
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
		b.WriteString(st.on + " " + st.off + " ")
		return
	}
	n := strconv.Itoa(i + 1)
	b.WriteString(st.on)
	b.WriteString(strings.Repeat(" ", gw-1-len(n)))
	b.WriteString(n)
	b.WriteString(st.off)
	b.WriteByte(' ')
}

// writeSpan writes bytes a to e of line i in the colors of their tokens,
// or of the matches over them.
func (m Model) writeSpan(b *strings.Builder, i, a, e int) {
	s := m.lines[i]
	var spans []span
	if i < len(m.spans) {
		spans = m.spans[i]
	}
	k, _ := slices.BinarySearchFunc(spans, a+1, func(x span, pos int) int { return x.end - pos })
	matches, first, _ := m.lineHits(i)
	cur := -1
	if m.search.cur >= 0 && i == m.search.curLine {
		cur = m.search.curNth
	}
	mi := 0
	for pos := a; pos < e; {
		next, p := e, m.esc.text
		for k < len(spans) && spans[k].end <= pos {
			k++
		}
		if k < len(spans) {
			next, p = min(next, spans[k].end), m.esc.token(spans[k].typ)
		}
		for mi < len(matches) && matches[mi][1] <= pos {
			mi++
		}
		if mi < len(matches) {
			start, end := matches[mi][0], matches[mi][1]
			switch {
			case start > pos:
				next = min(next, start)
			case first+mi == cur:
				next, p = min(next, end), m.esc.current
			default:
				next, p = min(next, end), m.esc.match
			}
		}
		b.WriteString(p.on)
		b.WriteString(s[pos:next])
		b.WriteString(p.off)
		pos = next
	}
}

// statusLine renders the name, or a note on the last search, on the left
// and where the window is on the right, or the search prompt while it is
// open.
func (m Model) statusLine() string {
	if m.prompt.Focused() {
		return fit(m.prompt.View(), m.width)
	}
	var parts []string
	switch s := m.search; {
	case s.running:
		parts = append(parts, m.esc.status.wrap("searching…"))
	case s.query != "":
		switch n := s.total(); {
		case s.cur < 0:
			parts = append(parts, m.esc.status.wrap(fmt.Sprintf("%d matches", n)))
		default:
			parts = append(parts, m.esc.status.wrap(fmt.Sprintf("match %d/%d", s.cur+1, n)))
		}
	}
	if n := len(m.lines); m.state == stateReady && n > 0 && m.bodyHeight() > 0 {
		pct := (m.bottom() + 1) * 100 / n
		parts = append(parts, m.esc.status.wrap(fmt.Sprintf("line %d/%d  %d%%", m.top+1, n, pct)))
	}
	left := m.nameView
	if m.flash != "" {
		left = m.esc.notice.wrap(m.flash)
	}
	right := strings.Join(parts, "  ")
	rw := ansi.StringWidth(right)
	if rw+2 > m.width {
		return fit(right, m.width)
	}
	return fit(left, m.width-rw-2) + "  " + right
}

// fit truncates or pads styled text to exactly width cells.
func fit(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		s = ansi.Truncate(s, width, "…")
		w = ansi.StringWidth(s)
	}
	if w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}
