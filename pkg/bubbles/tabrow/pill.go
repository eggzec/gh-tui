package tabrow

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// PillStyles makes the styles of a pill row from an accent style and a muted
// one: the tab on view is the accent in reverse video, bold.
func PillStyles(accent, muted lipgloss.Style) Styles {
	return Styles{
		Current: accent.Reverse(true).Bold(true),
		Other:   muted,
		Marker:  muted,
	}
}

// pilled draws the tabs as padded cells from the left edge. Where the labels
// don't fit the width it tries the short ones, and then only the tab on view,
// cut to the width if need be.
func (m Model) pilled() string {
	width := m.width - m.indent
	if width <= 0 {
		return ""
	}
	pad := strings.Repeat(" ", m.indent)
	cell := func(i int, short bool) string { return m.paint(i, " "+m.name(i, short)+" ") }
	for _, short := range []bool{false, true} {
		var b strings.Builder
		b.WriteString(pad)
		for i := range m.items {
			b.WriteString(cell(i, short))
		}
		if row := b.String(); ansi.StringWidth(row) <= m.width {
			return row
		}
	}
	return pad + ansi.Truncate(cell(min(max(m.cur, 0), len(m.items)-1), true), width, "")
}

// AlignWith lines the text of a pill row up with the text of the line under
// it, a table's header: the row is indented so that the first tab's label
// starts in the column where the header's first text does. The pill's solid
// cell then reaches one cell to the left of that column, the way its label
// is padded. Every pill row over a table gets its indent from here, so they
// agree. A header that starts at the left edge, or a blank one, leaves the
// row at the edge.
func (m *Model) AlignWith(header string) {
	plain := ansi.Strip(header)
	lead := len(plain) - len(strings.TrimLeft(plain, " "))
	if strings.TrimSpace(plain) == "" {
		lead = 0
	}
	m.indent = max(lead-1, 0)
}
