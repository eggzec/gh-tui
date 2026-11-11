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

// pilled draws the tabs as padded cells from the left edge. Where they don't
// fit the width, only the tab on view is drawn, cut to the width if need be.
func (m Model) pilled() string {
	cell := func(i int) string { return m.paint(i, " "+m.name(i, false)+" ") }
	var b strings.Builder
	for i := range m.items {
		b.WriteString(cell(i))
	}
	row := b.String()
	if ansi.StringWidth(row) > m.width {
		row = ansi.Truncate(cell(min(max(m.cur, 0), len(m.items)-1)), m.width, "")
	}
	return row
}
