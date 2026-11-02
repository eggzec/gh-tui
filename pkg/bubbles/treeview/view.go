package treeview

import (
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Fit returns lines as a block of exactly width cells and height lines:
// each line is cut or padded to width, and missing lines are blank. It is
// for what stands in the tree's place, such as a loading or error line. It
// returns "" when height is not positive.
func Fit(lines []string, width, height int) string {
	if height <= 0 {
		return ""
	}
	out := make([]string, height)
	for i := range out {
		var l string
		if i < len(lines) {
			l = lines[i]
		}
		out[i] = termtext.Fit(l, width, "")
	}
	return strings.Join(out, "\n")
}

// Render draws rows at width with no cursor and no window, every row in full
// and each folded as its Closed says, for hosts that only show a tree. It
// returns "" when there are no rows or no width.
func Render(rows []Node, width int, s Styles) string {
	m := New(Source{}, WithStyles(s))
	m.bare = true
	m.SetRows(rows)
	m.SetSize(width, m.starts[len(m.vis)])
	return m.View()
}

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
		lines = append(lines, m.empty()...)
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

// empty is the line shown in place of the rows: the spinner while the top
// level loads, its error if it failed, else the empty text, each with its hint
// on a line under it when it has one.
func (m Model) empty() []string {
	if _, ok := m.lz.busy[""]; ok {
		return []string{m.spinner() + m.styles.Empty.Render(m.cut(m.styles.LoadingText, max(m.width-2, 0)))}
	}
	if err := m.lz.failed[""]; err != nil {
		out := []string{m.styles.Error.Render(m.cut(m.errLine(err), m.width))}
		if h := m.errHint(err); h != "" {
			out = append(out, m.styles.Empty.Render(m.cut(h, m.width)))
		}
		return out
	}
	out := []string{m.styles.Empty.Render(m.cut(m.styles.EmptyText, m.width))}
	if h := m.styles.EmptyHint; h != "" {
		out = append(out, m.styles.Empty.Render(m.cut(h, m.width)))
	}
	return out
}

// spinner is the current frame and a space, or two spaces with no frames.
func (m Model) spinner() string {
	if len(m.look.spin) == 0 {
		return "  "
	}
	return m.look.spin[m.lz.frame%len(m.look.spin)] + " "
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
	if m.bare {
		glyph = ""
	}
	var text string
	if k == 0 {
		text = m.head(r, i)
	} else if room := m.width - m.gutter() - m.bodyw[i]; room > 0 {
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
func (m Model) head(r Node, i int) string {
	avail := m.width - m.gutter() - m.indent(i)
	var arrow string
	// Every row of a tree with a fold keeps a cell for the arrow, so that
	// siblings line up. An inline row hangs off the body of its parent,
	// which has none.
	if m.slot && !r.Inline {
		_, busy := m.lz.busy[r.ID]
		switch {
		case busy:
			arrow = m.spinner()
		case !m.foldAt(i):
			arrow = "  "
		case m.shut(m.vis[i]):
			arrow = m.look.closed + " "
		default:
			arrow = m.look.open + " "
		}
		avail -= 2
	}
	if avail <= 0 {
		return ""
	}

	// The right part comes last; it needs a column of text and a space.
	right, rw := r.Right, ansi.StringWidth(r.Right)
	room := avail
	if right != "" && avail-rw-1 >= m.minName {
		room = avail - rw - 1
	} else {
		right = ""
	}

	left := m.fit(r, i, room)
	gap := avail - ansi.StringWidth(left)
	if right != "" {
		gap -= rw
		right = m.styles.Right.Render(right)
	}
	return m.first[i] + arrow + left + strings.Repeat(" ", max(gap, 0)) + right
}

// fit draws the text of r, the row at place i of vis, with the marker of a
// closed fold in room cells. The text is cut with one ellipsis to keep the
// marker, unless that leaves the text fewer than 8 columns, and then the
// marker goes.
func (m Model) fit(r Node, i, room int) string {
	text := r.Lines[0]
	if m.icon != nil {
		if ic := m.icon(m.node(m.vis[i])); ic != "" {
			text = ic + " " + text
		}
	}
	var mark string
	var mw int
	if err := m.lz.failed[r.ID]; err != nil {
		e := m.errLine(err)
		mark = "  " + m.styles.Error.Render(e)
		mw = 2 + ansi.StringWidth(e)
	} else if r.Marker != "" && m.shut(m.vis[i]) {
		mark = "  " + m.styles.Marker.Render(r.Marker)
		mw = 2 + ansi.StringWidth(r.Marker)
	} else {
		return m.cut(text, room)
	}
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
