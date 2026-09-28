package finder

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// View renders the finder at exactly its width and height: the input, the
// matches below it, and a status line with their count.
func (m Model) View() string { return m.view }

// listHeight returns the number of rows for matches.
func (m Model) listHeight() int {
	return max(m.height-2, 0)
}

// layout sizes the input to the width, then renders.
func (m *Model) layout() {
	m.rows = nil
	// The input draws one cell more than its width, for the cursor.
	m.input.SetWidth(max(m.width-ansi.StringWidth(promptGlyph)-1, 1))
	m.input.SetCursor(m.input.Position())
	m.scroll()
	m.render()
}

// render renders the view for the current state.
func (m *Model) render() {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		m.view = ""
		return
	}
	lines := make([]string, 0, h)
	lines = append(lines, fit(m.esc.prompt+m.input.View(), w))
	if h > 1 {
		lines = m.appendList(lines, w, h-2)
		lines = append(lines, m.statusLine(w))
	}
	m.view = strings.Join(lines, "\n")
}

// statusLine renders how many paths match, and the note of the listing.
func (m *Model) statusLine(w int) string {
	var b strings.Builder
	switch {
	case m.loading:
		b.WriteString(m.spin.View())
		b.WriteString(m.esc.status.wrap("Loading " + m.many + "…"))
	case m.err != nil:
		return fit("", w)
	default:
		if m.matching {
			b.WriteString(m.spin.View())
		}
		b.WriteString(m.esc.status.wrap(m.count(m.Total(), m.one, m.many)))
		if m.res != nil && !m.res.all {
			b.WriteString(m.esc.status.wrap(" · " + m.count(m.Matches(), "match", "matches")))
		}
	}
	if m.note != "" {
		b.WriteString(m.esc.status.wrap(" · "))
		b.WriteString(m.esc.note.wrap(clean(m.note)))
	}
	return fit(b.String(), w)
}

func (m *Model) count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// appendList appends n lines of matches, or the state that stands in for
// them.
func (m *Model) appendList(lines []string, w, n int) []string {
	if n <= 0 {
		return lines
	}
	end := len(lines) + n
	st := m.styles
	switch {
	case m.err != nil:
		if text, hint := m.errorWords(m.err); text != "" {
			lines = append(lines, m.errorLine(text, hint, w))
		}
	case m.loading:
	case m.Total() == 0:
		lines = append(lines, fit(st.Empty.Render("No "+m.many+" here."), w))
	case m.Matches() == 0 && !m.matching:
		lines = append(lines, fit(st.Empty.Render("No "+m.many+" match. Try fewer letters."), w))
	case m.res != nil:
		ts := make([][]byte, len(m.res.terms))
		for i, t := range m.res.terms {
			ts[i] = []byte(t)
		}
		if m.rows == nil || len(m.rows) > 8*n {
			// Scrolling far keeps only the rows around the window.
			m.rows = make(map[int32]string, n)
		}
		for i := m.top; i < len(m.res.items) && len(lines) < end; i++ {
			it := m.res.items[i]
			if i == m.sel {
				lines = append(lines, m.row(it, true, ts, w))
				continue
			}
			r, ok := m.rows[it]
			if !ok {
				r = m.row(it, false, ts, w)
				m.rows[it] = r
			}
			lines = append(lines, r)
		}
	}
	blank := strings.Repeat(" ", w)
	for len(lines) < end {
		lines = append(lines, blank)
	}
	return lines
}

// errorWords returns what the finder says of err, the failed load, and
// the hint after it.
func (m *Model) errorWords(err error) (text, hint string) {
	if m.errorText != nil {
		return m.errorText(err)
	}
	msg, _, _ := strings.Cut(err.Error(), "\n")
	return "Couldn't list the " + m.many + ": " + clean(msg), ""
}

// errorLine renders the error row in w cells, the text cut to keep the
// hint whole.
func (m *Model) errorLine(text, hint string, w int) string {
	text = m.styles.ErrorGlyph + " " + text
	if hint == "" {
		return fit(m.styles.Error.Render(text), w)
	}
	hint = " · " + hint
	if room := max(w-ansi.StringWidth(hint), 0); ansi.StringWidth(text) > room {
		text = ansi.Truncate(text, room, "…")
	}
	return fit(m.styles.Error.Render(text)+m.styles.Status.Render(hint), w)
}

// minPath is the fewest cells a path keeps before the detail is dropped.
const minPath = 16

// row renders the item i, with the characters that match ts marked.
func (m *Model) row(i int32, selected bool, ts [][]byte, w int) string {
	it := &m.corpus.items[i]
	gutter, name := "  ", m.esc.name
	if selected {
		gutter, name = m.esc.gutterOn, m.esc.selName
	}
	icon := m.icon(it, w)
	iw := ansi.StringWidth(icon)
	room := w - 2 - iw
	path, detail := clean(it.Path), clean(it.Detail)
	dw := ansi.StringWidth(detail)
	pw := ansi.StringWidth(path)
	switch {
	case detail == "":
	case pw+2+dw <= room:
	case room-2-dw >= minPath:
	default:
		detail, dw = "", 0
	}
	if detail != "" {
		room -= dw + 2
	}
	var marks []bool
	if path == it.Path {
		// The offsets of the matches are those of the path as it is.
		marks = m.marks(i, ts)
	}
	var b strings.Builder
	b.Grow(w + 64)
	b.WriteString(gutter)
	b.WriteString(icon)
	var used int
	if m.links == nil {
		used = iw + writePath(&b, path, marks, room, m.esc.dir, name, m.esc.match)
	} else {
		var p strings.Builder
		used = iw + writePath(&p, path, marks, room, m.esc.dir, name, m.esc.match)
		b.WriteString(termtext.Link(m.links(*it), p.String()))
	}
	if detail != "" {
		b.WriteString(strings.Repeat(" ", w-2-used-dw))
		b.WriteString(m.esc.detail.wrap(detail))
		return b.String()
	}
	b.WriteString(strings.Repeat(" ", max(w-2-used, 0)))
	return b.String()
}

// icon returns the icon of it and the space after it, or "" when there is
// none or no room for it beside a cell of the path.
func (m *Model) icon(it *Item, w int) string {
	if m.icons == nil {
		return ""
	}
	ic := m.icons(*it)
	if ic == "" {
		return ""
	}
	ic += " "
	if ansi.StringWidth(ic) > w-3 {
		return ""
	}
	return ic
}

// marks returns whether each byte of the path of item i matches one of
// ts, or nil if none do.
func (m *Model) marks(i int32, ts [][]byte) []bool {
	if len(ts) == 0 {
		return nil
	}
	path, bonus := m.corpus.path(i)
	marks := make([]bool, len(path))
	for _, t := range ts {
		_, pos, ok := align(t, path, bonus)
		if !ok {
			continue
		}
		for _, p := range pos {
			marks[p] = true
		}
	}
	return marks
}

// writePath writes path in at most room cells, cut from the left where it
// is too long, so that the file name stays: dirs in dir, the name in name,
// and the bytes marked in match. It returns the cells written.
func writePath(b *strings.Builder, path string, marks []bool, room int, dir, name, match pair) int {
	if room <= 0 {
		return 0
	}
	cut, prefix := trim(path, room)
	used := 0
	if prefix != "" {
		b.WriteString(dir.wrap(prefix))
		used += ansi.StringWidth(prefix)
	}
	base := strings.LastIndexByte(path, '/') + 1
	for pos := cut; pos < len(path); {
		st := dir
		if pos >= base {
			st = name
		}
		marked := len(marks) > pos && marks[pos]
		if marked {
			st = match
		}
		end := pos + 1
		for end < len(path) && (len(marks) > end && marks[end]) == marked && (end >= base) == (pos >= base) {
			end++
		}
		// A run never splits a rune: the bytes of one are all marked or
		// none are, since only ASCII bytes fold to match a query.
		b.WriteString(st.on)
		b.WriteString(path[pos:end])
		b.WriteString(st.off)
		pos = end
	}
	return used + ansi.StringWidth(path[cut:])
}

// trim returns where to start path so that it fits in room cells, and the
// ellipsis that stands for what is cut: the path keeps as much of its end
// as fits, which names the file and the directories nearest it.
func trim(path string, room int) (cut int, prefix string) {
	if ansi.StringWidth(path) <= room {
		return 0, ""
	}
	for i := range len(path) {
		// One cell goes to the ellipsis.
		if !isContinuation(path[i]) && ansi.StringWidth(path[i:]) < room {
			return i, "…"
		}
	}
	return len(path), ""
}

func isContinuation(c byte) bool { return c&0xc0 == 0x80 }

// clean puts text on one line without escape sequences, so it can't break
// the layout.
func clean(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return termtext.OneLine(s)
	}
	return strings.Join(strings.Fields(termtext.OneLine(s)), " ")
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
