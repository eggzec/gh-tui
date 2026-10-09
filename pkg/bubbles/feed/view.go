package feed

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// View renders the visible rows in exactly Height lines of Width cells.
func (m Model[T]) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	body := m.height
	foot := m.hasFooter()
	if foot {
		body--
	}
	w := lineWriter{width: m.width, height: body, ellipsis: m.styles.ErrorEllipsis, cut: m.styles.Ellipsis}
	// Rows carry styles, so leave room for escape sequences.
	w.b.Grow(m.height * (m.width + 32))

	inner := max(m.width-m.Gutter(), 0)
	for i := m.top; !w.full(); i++ {
		switch {
		case i < m.shown():
			m.writeItem(&w, i, inner)
		case i == m.shown() && m.hasStatus():
			text, hint := m.statusLine()
			w.status(m.gutter(false, false), text, hint)
			w.blank(m.itemHeight - 1)
		default:
			w.blank(m.height)
		}
	}
	if foot {
		w.height = m.height
		w.line(m.footer())
	}
	return w.b.String()
}

func (m Model[T]) writeItem(w *lineWriter, i, width int) {
	selected := i == m.sel
	item, ok := m.item(m.at(i))
	if !ok {
		m.writePending(w, i, m.gutter(selected, false))
		return
	}
	gutter := m.gutter(selected, m.marked(item))
	rest := m.render(item, selected, width)
	for range m.itemHeight {
		var line string
		line, rest, _ = strings.Cut(rest, "\n")
		w.line(gutter, line)
	}
}

// writePending writes a row whose chunk is not loaded: a placeholder while
// it is fetched again, or the error once, where the failed chunk comes into
// view, or only the retry key for a failure not worth telling.
func (m Model[T]) writePending(w *lineWriter, i int, gutter string) {
	i = m.at(i)
	c := m.chunkAt(i)
	if m.chunks[c].err != nil && (i == m.starts[c] || i == m.top) {
		w.status(gutter, m.errLine, m.errHint)
	} else {
		w.line(gutter, m.placeholder)
	}
	w.blank(m.itemHeight - 1)
}

// gutter returns what goes left of a row: the cursor if it is selected,
// and the mark if it is marked, in a feed whose rows can be marked.
func (m Model[T]) gutter(selected, marked bool) string {
	if m.canMark() {
		state := 2
		switch {
		case !selected:
		case m.focused:
			state = 0
		default:
			state = 1
		}
		if marked {
			return m.markGutters[state][1]
		}
		return m.markGutters[state][0]
	}
	switch {
	case !selected:
		return m.gutterNone
	case m.focused:
		return m.gutterFocused
	default:
		return m.gutterBlurred
	}
}

// statusLine returns the row that follows the items: loading, error or
// empty. The hint is kept whole when the text has to be truncated.
func (m Model[T]) statusLine() (text, hint string) {
	switch {
	case m.tail.err != nil:
		return m.errLine, m.errHint
	case m.tail.fetching:
		return m.spin.View() + m.loadingText, ""
	case m.filter != "":
		text := "No loaded row matches."
		if !m.done {
			text = "No loaded match. Move down for more."
		}
		return m.styles.Empty.Render(text), ""
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
	// ellipsis ends the text of a status line where it is cut, and cut
	// any other line.
	ellipsis, cut string
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
			p = termtext.Truncate(p, left, w.cut)
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
		text = termtext.Truncate(text, max(room, 0), w.ellipsis)
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
