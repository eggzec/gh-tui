package pager

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

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
	if m.state == stateReady && len(m.lines) > 0 {
		rows = m.writeLines(&b)
	} else if m.bodyHeight() > 0 {
		b.WriteString(fit(m.message(), m.width))
		b.WriteByte('\n')
		rows = 1
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
	case stateFailed:
		msg := "unknown error"
		if m.err != nil {
			msg, _, _ = strings.Cut(m.err.Error(), "\n")
		}
		return s.Error.Render("✗ Couldn't load: " + msg)
	case stateBinary:
		return s.Message.Render("Binary file, not shown.")
	case stateReady:
		return s.Message.Render("Empty file.")
	default:
		return s.Message.Render("Nothing to show.")
	}
}

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
	n := strconv.Itoa(i + 1)
	b.WriteString(m.esc.number.on)
	b.WriteString(strings.Repeat(" ", gw-1-len(n)))
	b.WriteString(n)
	b.WriteString(m.esc.number.off)
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
	matches, first := m.lineMatches(i)
	mi := 0
	for pos := a; pos < e; {
		next, p := e, m.esc.text
		for k < len(spans) && spans[k].end <= pos {
			k++
		}
		if k < len(spans) {
			next, p = min(next, spans[k].end), m.esc.token(spans[k].typ)
		}
		for mi < len(matches) && matches[mi].end <= pos {
			mi++
		}
		if mi < len(matches) {
			x := matches[mi]
			switch {
			case x.start > pos:
				next = min(next, x.start)
			case first+mi == m.search.cur:
				next, p = min(next, x.end), m.esc.current
			default:
				next, p = min(next, x.end), m.esc.match
			}
		}
		b.WriteString(p.on)
		b.WriteString(s[pos:next])
		b.WriteString(p.off)
		pos = next
	}
}

// statusLine renders the name on the left and where the window is on the
// right, or the search input while it is open.
func (m Model) statusLine() string {
	if m.searching {
		return fit(m.input.View(), m.width)
	}
	var parts []string
	if q := m.search.query; q != "" {
		if n := len(m.search.matches); n == 0 {
			parts = append(parts, m.esc.notice.wrap("no matches"))
		} else {
			parts = append(parts, m.esc.status.wrap(fmt.Sprintf("match %d/%d", m.search.cur+1, n)))
		}
	}
	if n := len(m.lines); m.state == stateReady && n > 0 && m.bodyHeight() > 0 {
		pct := (m.bottom() + 1) * 100 / n
		parts = append(parts, m.esc.status.wrap(fmt.Sprintf("line %d/%d  %d%%", m.top+1, n, pct)))
	}
	right := strings.Join(parts, "  ")
	rw := ansi.StringWidth(right)
	if rw+2 > m.width {
		return fit(right, m.width)
	}
	return fit(m.nameView, m.width-rw-2) + "  " + right
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
