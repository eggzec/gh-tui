package foldtree

import (
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// View draws the window of the tree: exactly Height lines of Width cells.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	lines := make([]string, 0, m.height)
	for _, p := range m.pins {
		lines = append(lines, m.pinned(p))
	}
	if len(m.vis) == 0 {
		lines = append(lines, m.styles.Empty.Render(m.cut(m.styles.EmptyText, m.width)))
	}
	i := sort.Search(len(m.vis), func(i int) bool { return m.starts[i+1] > m.off })
	for ; i < len(m.vis) && len(lines) < m.height; i++ {
		for k := max(m.off-m.starts[i], 0); k < m.lines(i) && len(lines) < m.height; k++ {
			lines = append(lines, m.render(i, k))
		}
	}
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	for i, l := range lines {
		lines[i] = m.pad(l, m.width)
	}
	return strings.Join(lines, "\n")
}

// render returns line k of the row at place i of vis. The cursor row of a
// focused tree has the background of CursorRow across it, resumed after each
// reset the row's own text makes, and a blurred one only the cursor glyph.
func (m Model) render(i, k int) string {
	r := m.row(i)
	glyph := " "
	if i == m.cur {
		glyph = m.look.blurred
		if m.focused {
			glyph = m.look.cursor
		}
	}
	var text string
	if k == 0 {
		text = m.head(r, i)
	} else if room := m.width - 1 - m.bodyw[i]; room > 0 {
		text = m.body[i] + m.cut(r.Lines[k], room)
	}
	line := glyph + text
	if i == m.cur && m.focused && m.look.bg != "" {
		line = m.shade(line, m.look.bg)
	}
	return line
}

// pinned returns the line of the row at place i of vis as a pin: on the
// background of the pins, unless it is the cursor row, which is drawn as such.
func (m Model) pinned(i int) string {
	line := m.render(i, 0)
	if i == m.cur || m.look.pin == "" {
		return line
	}
	return m.shade(line, m.look.pin)
}

// shade draws line on the background bg across the width.
func (m Model) shade(line, bg string) string {
	line = strings.ReplaceAll(line, "\x1b[m", "\x1b[m"+bg)
	line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+bg)
	if pad := m.width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return bg + line + "\x1b[m"
}

// head draws the first line of row r, the one at place i of vis, after its
// cursor column: guides, the arrow of a fold, the text, the marker of a
// closed fold and the right-aligned part, in the width that is left.
func (m Model) head(r Row, i int) string {
	avail := m.width - 1 - m.indent(i)
	var arrow string
	if r.Arrow {
		arrow = m.look.open
		if m.shut(r) {
			arrow = m.look.closed
		}
		arrow += " "
		avail -= 2
	}
	if avail <= 0 {
		return ""
	}

	// The right part comes last; it needs a column of text and a space.
	right, rw := r.Right, ansi.StringWidth(r.Right)
	room := avail
	if right != "" && avail-rw-1 >= 1 {
		room = avail - rw - 1
	} else {
		right = ""
	}

	left := m.fit(r, room)
	gap := avail - ansi.StringWidth(left)
	if right != "" {
		gap -= rw
	}
	return m.first[i] + arrow + left + strings.Repeat(" ", max(gap, 0)) + right
}

// fit draws the text of r with the marker of a closed fold in room cells.
// The text is cut with one ellipsis to keep the marker, unless that leaves
// the text fewer than 8 columns, and then the marker goes.
func (m Model) fit(r Row, room int) string {
	text := r.Lines[0]
	if r.Marker == "" || !m.shut(r) {
		return m.cut(text, room)
	}
	mark := "  " + m.styles.Marker.Render(r.Marker)
	mw := 2 + ansi.StringWidth(r.Marker)
	switch {
	case ansi.StringWidth(text)+mw <= room:
		return text + mark
	case room-mw >= 8:
		return m.cut(text, room-mw) + mark
	}
	return m.cut(text, room)
}

// cut cuts s to w cells with the ellipsis, when it is wider.
func (m Model) cut(s string, w int) string {
	if ansi.StringWidth(s) <= w {
		return s
	}
	return termtext.Truncate(s, w, m.styles.Ellipsis)
}

// pad pads s with spaces to w cells.
func (m Model) pad(s string, w int) string {
	if n := w - ansi.StringWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
