package calendar

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// View renders the calendar in exactly Height lines of Width cells.
func (m Model) View() string {
	return strings.Join(m.lines, "\n")
}

// render renders every line of the view.
func (m *Model) render() {
	if m.width <= 0 || m.height <= 0 {
		m.lines = nil
		return
	}
	m.lines = make([]string, m.height)
	blank := strings.Repeat(" ", m.width)
	for i := range m.lines {
		m.lines[i] = blank
	}
	if len(m.grid) == 0 {
		m.lines[0] = m.fit(m.styles.Empty.Render(m.emptyText))
		return
	}
	total := totalText(m.Total())
	if len(total) > m.width {
		total = contributions(m.Total(), "0 contributions")
	}
	m.setLine(lineTotal, m.fit(m.styles.Total.Render(total)))
	m.setLine(lineMonths, m.monthsLine())
	for d := range 7 {
		m.renderRow(d)
	}
	m.renderFooter()
}

// setLine sets line i of the view if the height has room for it.
func (m *Model) setLine(i int, s string) {
	if i < len(m.lines) {
		m.lines[i] = s
	}
}

// renderRow renders the row of weekday d.
func (m *Model) renderRow(d int) {
	if lineDays+d >= len(m.lines) {
		return
	}
	var b strings.Builder
	// Each day carries its style, so leave room for escape sequences.
	b.Grow(m.width + m.cols*len(m.frags[Levels-1]))
	used := 0
	if m.weekdays {
		b.WriteString(m.dayLabels[d])
		used += gutterW
	}
	for c := range m.cols {
		if c > 0 {
			b.WriteByte(' ')
			used++
		}
		w := m.start + c
		s := m.grid[w][d]
		switch {
		case !s.ok:
			b.WriteByte(' ')
		case m.focused && w == m.cw && d == m.cd:
			b.WriteString(m.cursorFrags[s.day.Level])
		default:
			b.WriteString(m.frags[s.day.Level])
		}
		used++
	}
	pad(&b, m.width-used)
	m.lines[lineDays+d] = b.String()
}

// monthsLine renders the month labels above the weeks shown.
func (m Model) monthsLine() string {
	var b strings.Builder
	used := 0
	if m.weekdays {
		pad(&b, gutterW)
		used = gutterW
	}
	origin := used
	for _, l := range monthLabels(m.grid[m.start:m.start+m.cols], m.width-origin) {
		pad(&b, origin+cellW*l.col-used)
		b.WriteString(m.styles.Month.Render(l.name))
		used = origin + cellW*l.col + len(l.name)
	}
	pad(&b, m.width-used)
	return b.String()
}

// renderFooter renders the line below the grid: the day under the cursor
// while focused on the left, and the legend on the right. The legend is
// dropped first when both do not fit.
func (m *Model) renderFooter() {
	if lineFooter >= len(m.lines) {
		return
	}
	status, statusW := "", 0
	if day, ok := m.Selected(); ok && m.focused {
		text := statusText(day)
		status, statusW = m.styles.Status.Render(text), ansi.StringWidth(text)
	}
	gap := 0
	if statusW > 0 {
		gap = 1
	}
	if statusW+gap+m.legendW > m.width {
		m.lines[lineFooter] = m.fit(status)
		return
	}
	var b strings.Builder
	b.WriteString(status)
	pad(&b, m.width-statusW-m.legendW)
	b.WriteString(m.legend)
	m.lines[lineFooter] = b.String()
}

// fit truncates or pads s to the width.
func (m Model) fit(s string) string {
	w := ansi.StringWidth(s)
	if w > m.width {
		s = ansi.Truncate(s, m.width, "…")
		w = ansi.StringWidth(s)
	}
	return s + strings.Repeat(" ", m.width-w)
}

func pad(b *strings.Builder, n int) {
	for range n {
		b.WriteByte(' ')
	}
}
