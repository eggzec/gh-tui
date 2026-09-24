package tree

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// View renders the visible rows in exactly Height lines of Width cells.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	w := lineWriter{width: m.width, height: m.height}
	// Rows carry styles, so leave room for escape sequences.
	w.b.Grow(m.height * (m.width + 48))

	for i := m.top; !w.full(); i++ {
		switch {
		case i < len(m.rows):
			m.writeRow(&w, i)
		case i == len(m.rows) && m.hasStatus():
			text, hint := m.statusLine()
			w.status(m.gutterNone, text, hint)
		default:
			w.blank(m.height)
		}
	}
	return w.b.String()
}

func (m Model) writeRow(w *lineWriter, i int) {
	e := m.rows[i]
	marker := m.markerLeaf
	if e.node.Branch {
		switch {
		case e.loading:
			marker = m.spin.View() + " "
		case e.expanded:
			marker = m.markerOpen
		default:
			marker = m.markerClosed
		}
	}
	icon := ""
	if m.icons != nil {
		if ic := m.icons(e.node, e.expanded); ic != "" {
			icon = ic + " "
		}
	}
	prefix := m.gutter(i == m.sel)
	if e.err == nil {
		m.writeName(w, e, prefix, marker, icon)
		return
	}
	// Keep the retry hint whole, and share the rest between the name and
	// the error.
	guide := m.guides[e.depth]
	room := w.width - ansi.StringWidth(prefix) - ansi.StringWidth(guide) -
		ansi.StringWidth(marker) - ansi.StringWidth(icon) - ansi.StringWidth(m.errHint)
	name, msg := e.label, " "+m.errText(e.err)
	nw, mw := ansi.StringWidth(name), ansi.StringWidth(msg)
	if nw+mw > room {
		if nw > room/2 {
			name = ansi.Truncate(name, max(room/2, 1), "…")
			nw = ansi.StringWidth(name)
		}
		msg = ansi.Truncate(msg, max(room-nw, 0), "…")
	}
	w.line(prefix, guide, marker, icon, name, msg, m.errHint)
}

// minDetailName is the fewest cells of a name that a detail may leave.
const minDetailName = 10

// writeName writes a row that loaded, with its detail at the right edge
// when there is room for it. The name is truncated before the detail is
// dropped, but never below minDetailName cells.
func (m Model) writeName(w *lineWriter, e *entry, prefix, marker, icon string) {
	guide := m.guides[e.depth]
	if e.detail == "" {
		w.line(prefix, guide, marker, icon, e.label)
		return
	}
	room := w.width - ansi.StringWidth(prefix) - ansi.StringWidth(guide) -
		ansi.StringWidth(marker) - ansi.StringWidth(icon)
	dw := ansi.StringWidth(e.detail)
	nw := ansi.StringWidth(e.label)
	// One space keeps the name and the detail apart.
	avail := room - dw - 1
	if avail < min(nw, minDetailName) {
		w.line(prefix, guide, marker, icon, e.label)
		return
	}
	name := e.label
	if nw > avail {
		name = ansi.Truncate(name, avail, "…")
		nw = ansi.StringWidth(name)
	}
	w.right(e.detail, room-nw-dw, prefix, guide, marker, icon, name)
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

func (m Model) errText(err error) string {
	msg, _, _ := strings.Cut(err.Error(), "\n")
	return m.styles.Error.Render("✗ " + msg)
}

// statusLine returns the row shown after the rows, or instead of them:
// loading, error or empty. The hint is kept whole when the text has to be
// truncated.
func (m Model) statusLine() (text, hint string) {
	root := m.nodes[""]
	switch {
	case root.err != nil:
		msg, _, _ := strings.Cut(root.err.Error(), "\n")
		return m.styles.Error.Render("✗ Couldn't load: " + msg), m.errHint
	case root.loading:
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
}

func (w *lineWriter) full() bool {
	return w.lines >= w.height
}

// line writes one line made of parts.
func (w *lineWriter) line(parts ...string) {
	if w.full() {
		return
	}
	if w.lines > 0 {
		w.b.WriteByte('\n')
	}
	w.lines++
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

// right writes parts, then gap spaces, then detail. The parts and the
// detail must fit the width with the gap.
func (w *lineWriter) right(detail string, gap int, parts ...string) {
	if w.full() {
		return
	}
	if w.lines > 0 {
		w.b.WriteByte('\n')
	}
	w.lines++
	for _, p := range parts {
		w.b.WriteString(p)
	}
	w.pad(gap)
	w.b.WriteString(detail)
}

// status writes prefix, then text truncated to leave room for hint, then
// hint.
func (w *lineWriter) status(prefix, text, hint string) {
	room := w.width - ansi.StringWidth(prefix) - ansi.StringWidth(hint)
	if tw := ansi.StringWidth(text); tw > room {
		text = ansi.Truncate(text, max(room, 0), "…")
	}
	w.line(prefix, text, hint)
}

// blank writes n empty lines.
func (w *lineWriter) blank(n int) {
	for range n {
		if w.full() {
			return
		}
		if w.lines > 0 {
			w.b.WriteByte('\n')
		}
		w.lines++
		w.pad(w.width)
	}
}

func (w *lineWriter) pad(n int) {
	for range n {
		w.b.WriteByte(' ')
	}
}
