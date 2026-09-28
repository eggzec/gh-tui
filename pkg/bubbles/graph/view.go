package graph

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// minText is the fewest cells of the text that the right column may leave.
const minText = 12

// View renders the visible rows in exactly Height lines of Width cells.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	w := lineWriter{width: m.width, height: m.height, ellipsis: m.styles.ErrorEllipsis}
	// Rows carry styles, so leave room for escape sequences.
	w.b.Grow(m.height * (m.width + 96))

	end := min(m.top+m.height, len(m.rows))
	// Line up the text of the visible rows after the widest graph.
	graphW := 0
	for i := m.top; i < end; i++ {
		graphW = max(graphW, 2*len(m.rows[i].cells)-1)
	}
	for i := m.top; !w.full(); i++ {
		switch {
		case i < len(m.rows):
			m.writeRow(&w, i, graphW)
		case i == len(m.rows) && m.hasStatus():
			text, hint := m.statusLine()
			w.status(m.gutterNone, text, hint)
		default:
			w.blank(m.height)
		}
	}
	return w.b.String()
}

// writeRow writes commit i with its graph padded to graphW cells, then its
// text, and its right column at the right edge when there is room for it.
// The text is truncated before the right column is dropped, but never below
// minText cells.
func (m Model) writeRow(w *lineWriter, i, graphW int) {
	r := &m.rows[i]
	room := w.width - gutterWidth - graphW - 1
	if room < 0 {
		// Too narrow even for the graph: render it whole and cut the line.
		var b strings.Builder
		m.writeGraph(&b, r.cells, graphW)
		w.line(m.gutter(i == m.sel), b.String(), " ", r.text)
		return
	}
	text, textW := r.text, r.textW
	showRight := r.rightW > 0 && room-r.rightW > min(textW, minText)
	avail := room
	if showRight {
		avail -= r.rightW + 1
	}
	if textW > avail {
		text = ansi.Truncate(text, avail, "…")
		textW = ansi.StringWidth(text)
	}

	b := w.begin()
	b.WriteString(m.gutter(i == m.sel))
	m.writeGraph(b, r.cells, graphW)
	b.WriteByte(' ')
	b.WriteString(text)
	if showRight {
		w.pad(room - textW - r.rightW)
		b.WriteString(r.right)
		return
	}
	w.pad(room - textW)
}

// writeGraph writes the cells of a row and pads them to width.
func (m Model) writeGraph(b *strings.Builder, cells []cell, width int) {
	lanes := len(m.hline)
	for j, c := range cells {
		b.WriteString(m.frags[c.glyph][int(c.color)%lanes])
		if j == len(cells)-1 {
			break
		}
		if c.gap == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteString(m.hline[int(c.gap-1)%lanes])
		}
	}
	for range width - max(2*len(cells)-1, 0) {
		b.WriteByte(' ')
	}
}

func (m Model) gutter(selected bool) string {
	switch {
	case !selected:
		return m.gutterNone
	case m.focused:
		return m.gutterFocused
	default:
		return m.gutterBlurred
	}
}

// statusLine returns the row that follows the commits: loading, error or
// empty. The hint is kept whole when the text has to be truncated.
func (m Model) statusLine() (text, hint string) {
	switch {
	case m.err != nil:
		return m.errLine, m.errHint
	case m.fetching:
		return m.spin.View() + m.loadingText, ""
	default:
		return m.emptyLine, ""
	}
}

// lineWriter writes lines that are truncated or padded to width, and stops
// at height.
type lineWriter struct {
	b      strings.Builder
	width  int
	height int
	lines  int
	// ellipsis ends the text of a status line where it is cut.
	ellipsis string
}

func (w *lineWriter) full() bool {
	return w.lines >= w.height
}

// begin starts a line that the caller writes to fit the width exactly.
func (w *lineWriter) begin() *strings.Builder {
	if w.lines > 0 {
		w.b.WriteByte('\n')
	}
	w.lines++
	return &w.b
}

// line writes one line made of parts.
func (w *lineWriter) line(parts ...string) {
	if w.full() {
		return
	}
	w.begin()
	left := w.width
	for _, p := range parts {
		pw := ansi.StringWidth(p)
		if pw > left {
			p = ansi.Truncate(p, left, "…")
			pw = ansi.StringWidth(p)
			w.b.WriteString(p)
			w.pad(left - pw)
			return
		}
		w.b.WriteString(p)
		left -= pw
	}
	w.pad(left)
}

// status writes prefix, then text truncated to leave room for hint, then
// hint.
func (w *lineWriter) status(prefix, text, hint string) {
	room := w.width - ansi.StringWidth(prefix) - ansi.StringWidth(hint)
	if tw := ansi.StringWidth(text); tw > room {
		text = ansi.Truncate(text, max(room, 0), w.ellipsis)
	}
	w.line(prefix, text, hint)
}

// blank writes n empty lines.
func (w *lineWriter) blank(n int) {
	for range n {
		if w.full() {
			return
		}
		w.begin()
		w.pad(w.width)
	}
}

func (w *lineWriter) pad(n int) {
	for range n {
		w.b.WriteByte(' ')
	}
}
