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
		w.line(prefix, m.guides[e.depth], marker, icon, e.label)
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
