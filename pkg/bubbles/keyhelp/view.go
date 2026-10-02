package keyhelp

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Widths of the columns of a row: the mark, the keys, and at most the
// source. The description fills the rest.
const (
	markWidth   = 2
	keysWidth   = 14
	sourceWidth = 16
	// sourceMinWidth is the narrowest width that still shows the source.
	sourceMinWidth = 50
)

// View renders the help at exactly its width and height: the title, the
// query line, and the rows that match below. A help one row high keeps
// the query line.
func (m Model) View() string {
	if m.stale {
		// A copy, so the rows it renders don't go into the shared cache.
		m.drawn = nil
		m.relist()
	}
	return m.view
}

// columns returns the widths of the keys and the source columns at the
// current width, each with the gap after the column before it.
func (m Model) columns() (keys, source int) {
	w := m.width
	keys = min(keysWidth, max(w-markWidth, 0)/3)
	if w >= sourceMinWidth {
		for _, l := range m.layers {
			source = max(source, ansi.StringWidth(clean(l.Source))+1)
		}
		source = min(source, sourceWidth)
	}
	return keys, source
}

// list lists the rows shown now if the help is open, or else when it
// opens.
func (m *Model) list() {
	if !m.focused {
		m.stale = true
		return
	}
	m.relist()
}

// relist renders the rows shown into the viewport, at the current width
// and styles.
func (m *Model) relist() {
	m.stale = false
	w := m.width
	m.vp.SetWidth(w)
	m.vp.SetHeight(max(m.height-2, 0))
	if len(m.shown) == 0 {
		m.vp.SetContentLines([]string{m.fit(m.styles.Empty.Render(m.emptyText), w)})
		m.render()
		return
	}
	if m.drawnWidth != w || len(m.drawn) != len(m.rows) {
		m.drawn, m.drawnWidth = make([][]string, len(m.rows)), w
	}
	kw, sw := -1, -1
	lines := make([]string, 0, len(m.shown)+4)
	for i, ri := range m.shown {
		r := m.rows[ri]
		if i > 0 && m.rows[m.shown[i-1]].Layer != r.Layer {
			lines = append(lines, strings.Repeat(" ", w))
		}
		if m.drawn[ri] == nil {
			if kw < 0 {
				kw, sw = m.columns()
			}
			drawn := make([]string, 0, 1+len(r.Lost))
			drawn = append(drawn, m.row(r, w, kw, sw))
			for _, l := range r.Lost {
				drawn = append(drawn, m.loss(l, w, kw))
			}
			m.drawn[ri] = drawn
		}
		lines = append(lines, m.drawn[ri]...)
	}
	m.vp.SetContentLines(lines)
	m.render()
}

// row renders r at width w, with keys and source columns kw and sw wide.
func (m *Model) row(r Row, w, kw, sw int) string {
	s := m.styles
	mark, keys, desc, src := s.Conflict, s.Key, s.Desc, s.Source
	switch r.Status {
	case Active, Conflict:
	case Shadowed:
		mark = s.Shadowed
	case Disabled, Typed:
		keys, desc, src = s.Disabled, s.Disabled, s.Disabled
	}
	glyph := "  "
	if r.Status == Conflict || r.Status == Shadowed {
		glyph = mark.Render(termtext.Cells(s.WarnGlyph, 1)) + " "
	}
	dw := max(w-markWidth-kw-sw, 0)
	var b strings.Builder
	b.WriteString(glyph)
	b.WriteString(m.column(keys, strings.Join(r.Binding.Keys(), " "), kw))
	b.WriteString(m.column(desc, r.Binding.Help().Desc, dw))
	if sw > 0 {
		b.WriteString(m.column(src, r.Source, sw))
	}
	return m.fit(b.String(), w)
}

// loss renders the line under a row that says who gets key l, indented to
// the description.
func (m *Model) loss(l Loss, w, kw int) string {
	st := m.styles.Conflict
	text := m.styles.LossGlyph + " " + l.Key + ": " + l.By.Help().Desc + m.styles.Separator + l.Source
	switch l.Status {
	case Active, Disabled, Conflict:
	case Shadowed:
		st = m.styles.Shadowed
	case Typed:
		st = m.styles.Typed
		text = m.styles.LossGlyph + " " + l.Key + ": typed in " + l.Source
	}
	return m.fit(strings.Repeat(" ", min(markWidth+kw, w))+st.Render(clean(text)), w)
}

// render renders the view for the current state.
func (m *Model) render() {
	w, h := m.width, m.height
	if w <= 0 || h <= 0 {
		m.view = ""
		return
	}
	lines := make([]string, 0, 3)
	if h >= 2 {
		lines = append(lines, m.titleLine(w))
	}
	lines = append(lines, m.queryLine(w))
	if h >= 3 {
		lines = append(lines, m.vp.View())
	}
	m.view = strings.Join(lines, "\n")
}

// titleLine renders the title, and how many rows are shown on the right.
func (m *Model) titleLine(w int) string {
	var count string
	switch n := strconv.Itoa(len(m.shown)); {
	case m.filtered():
		count = n + " of " + strconv.Itoa(len(m.rows))
	case n == "1":
		count = "1 key"
	default:
		count = n + " keys"
	}
	title := m.styles.Title.Render(clean(m.title))
	gap := w - ansi.StringWidth(title) - len(count)
	if gap < 1 {
		return m.fit(title, w)
	}
	return title + strings.Repeat(" ", gap) + m.styles.Count.Render(count)
}

// queryLine renders the query, or what to do while a key is captured.
func (m *Model) queryLine(w int) string {
	if m.capturing {
		text := "Press a key to find it" + m.styles.Separator + "tab to stop"
		if m.key != "" {
			text = "Key " + m.key + m.styles.Separator + "press another, or tab to stop"
		}
		return m.fit(m.styles.Capture.Render(clean(text)), w)
	}
	head := m.prompt
	if m.key != "" {
		head += m.styles.Capture.Render(clean("key "+m.key)) + " "
	}
	// The input draws one cell more than its width, for the cursor.
	m.input.SetWidth(max(w-ansi.StringWidth(head)-1, 1))
	return m.fit(head+m.input.View(), w)
}

// column renders text in st, cut to leave a gap and padded to width.
func (m *Model) column(st lipgloss.Style, text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = clean(text)
	if ansi.StringWidth(text) > width-1 {
		text = termtext.Truncate(text, max(width-1, 0), m.styles.Ellipsis)
	}
	return st.Render(text) + strings.Repeat(" ", width-ansi.StringWidth(text))
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

// clean puts text on one line without escape sequences, control or
// invisible format characters, so it can't break the layout or command
// the terminal.
func clean(s string) string {
	return strings.Join(strings.Fields(termtext.OneLine(s)), " ")
}
