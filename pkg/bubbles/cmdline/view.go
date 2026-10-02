package cmdline

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// View renders the command line at exactly its width and [Model.Height]:
// the candidate row, when there are candidates, over the prompt and the
// line, which scrolls so the cursor is in view.
func (m Model) View() string { return m.view }

// layout sizes the input to the room after the prompt, then renders.
func (m *Model) layout() {
	// The input draws one cell more than its width, for the cursor.
	if w := max(m.width-m.promptWidth-1, 1); w != m.input.Width() {
		m.input.SetWidth(w)
		// Scroll the input again for the new width. The input scrolls
		// only when the cursor leaves the part it shows, which a
		// narrower input may no longer reach, so the cursor goes to the
		// end and back. At the same width, the scroll stays as it is.
		pos := m.input.Position()
		m.input.CursorEnd()
		m.input.SetCursor(pos)
	}
	// Keep the selected candidate in view, and fill a wider row.
	m.scroll()
	m.widen()
	m.render()
}

// render renders the view for the current state.
func (m *Model) render() {
	if m.Height() == 0 {
		m.view = ""
		return
	}
	line := m.fit(m.promptView+m.inputView(), m.width)
	if m.Height() == 1 {
		m.view = line
		return
	}
	m.view = m.rowView() + "\n" + line
}

// inputView returns the input's view. The input pads a placeholder with
// wide characters with NUL bytes, which draw nothing, so they go and fit
// pads the line instead.
func (m *Model) inputView() string {
	v := m.input.View()
	if m.input.Value() == "" && m.input.Placeholder != "" {
		v = strings.ReplaceAll(v, "\x00", "")
	}
	return v
}

// renderItems renders each candidate, plain and selected, for the row.
func (m *Model) renderItems() {
	c, st := &m.comp, m.styles
	c.items = c.items[:0:0]
	for _, cd := range c.cands {
		label := clean(cd.Label)
		if label == "" {
			label = clean(cd.Text)
		}
		plain, sel := st.Candidate.Render(" "+label), " "+label
		if detail := clean(cd.Detail); detail != "" {
			plain += st.Detail.Render(" " + detail)
			sel += " " + detail
		}
		sel = st.Selected.Render(sel + " ")
		c.items = append(c.items, item{plain: plain + " ", selected: sel, width: ansi.StringWidth(sel)})
	}
	c.rowWidth = -1
}

// fitRow returns the end of the candidates the row shows from first, and
// whether more follow. The row keeps its first column for a mark that it
// scrolled, and its last for one that more follow. It shows at least one
// candidate, cut if it must be.
func (m *Model) fitRow(first int) (end int, more bool) {
	items := m.comp.items
	room := m.width - 1
	end = fill(items, first, room)
	if end < len(items) {
		end = fill(items, first, room-1)
	}
	end = max(end, min(first+1, len(items)))
	return end, end < len(items)
}

// fill returns the end of the items from first that fit in room, one
// space apart.
func fill(items []item, first, room int) int {
	used := 0
	for i := first; i < len(items); i++ {
		w := items[i].width
		if i > first {
			w++
		}
		if used+w > room {
			return i
		}
		used += w
	}
	return len(items)
}

// rowView returns the candidate row, rendered again only when the
// candidates, the width or the selection changed.
func (m *Model) rowView() string {
	c := &m.comp
	if c.rowWidth == m.width && c.rowSel == c.sel && c.rowFirst == c.first {
		return c.row
	}
	end, more := m.fitRow(c.first)
	var b strings.Builder
	if c.first > 0 {
		b.WriteString(m.moreLeft)
	} else {
		b.WriteByte(' ')
	}
	// The widths are known, so only a candidate cut to fit needs measuring.
	used := 1
	for i := c.first; i < end; i++ {
		if i > c.first {
			b.WriteByte(' ')
			used++
		}
		if i == c.sel {
			b.WriteString(c.items[i].selected)
		} else {
			b.WriteString(c.items[i].plain)
		}
		used += c.items[i].width
	}
	room := m.width
	if more {
		room--
	}
	row := b.String()
	if used > room {
		row = m.fit(row, max(room, 0))
	} else {
		row += strings.Repeat(" ", room-used)
	}
	if more {
		row += m.moreRight
	}
	c.row = row
	c.rowWidth, c.rowSel, c.rowFirst = m.width, c.sel, c.first
	return c.row
}

// fit truncates styled text to exactly width cells, ending it with the
// ellipsis where it cuts, or pads it.
func (m *Model) fit(s string, width int) string {
	w := ansi.StringWidth(s)
	if w > width {
		s = termtext.Truncate(s, width, m.styles.Ellipsis)
		w = ansi.StringWidth(s)
	}
	if w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

// clean puts text on one line without escape sequences, so it can't break
// the layout.
func clean(s string) string {
	return strings.Join(strings.Fields(termtext.OneLine(s)), " ")
}

// oneLine puts text on one line without escape sequences, keeping its
// spaces.
func oneLine(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(ansi.Strip(s))
}
