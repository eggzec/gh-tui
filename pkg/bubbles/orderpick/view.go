package orderpick

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// SetSize sets the width and height of both panes together, frames included.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	lw, lh, _, _ := m.sizes()
	m.left.SetSize(lw, lh)
	m.scroll()
}

// Width returns the width.
func (m Model) Width() int { return m.width }

// Height returns the height.
func (m Model) Height() int { return m.height }

// sizes returns the size of each pane: side by side, or stacked when the
// width is narrow.
func (m Model) sizes() (lw, lh, rw, rh int) {
	if m.width < narrow {
		lh = (m.height + 1) / 2
		return m.width, lh, m.width, m.height - lh
	}
	lw = m.width / 2
	return lw, m.height, m.width - lw, m.height
}

// frame returns the frame of the right pane.
func (m Model) frame() lipgloss.Style {
	if m.pane == PaneChosen {
		return m.styles.FocusFrame
	}
	return m.styles.Frame
}

// inner returns the size inside the frame of the right pane.
func (m Model) inner() (width, height int) {
	_, _, rw, rh := m.sizes()
	f := m.frame()
	return rw - f.GetHorizontalFrameSize(), rh - f.GetVerticalFrameSize()
}

// rows returns the number of rows of the right pane for chosen items: those
// inside its frame, but for its heading and its count.
func (m Model) rows() int {
	_, h := m.inner()
	return max(h-2, 0)
}

// View renders both panes at exactly the width and height.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	right := m.rightView()
	if m.width < narrow {
		if right == "" {
			return m.left.View()
		}
		return lipgloss.JoinVertical(lipgloss.Left, m.left.View(), right)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.left.View(), right)
}

// rightView renders the chosen items inside their frame.
func (m Model) rightView() string {
	_, _, rw, rh := m.sizes()
	w, h := m.inner()
	if rh <= 0 {
		return ""
	}
	if w < 1 || h < 1 {
		return strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", max(rw, 0))+"\n", max(rh, 0)), "\n")
	}
	st := m.styles
	lines := make([]string, 0, h)
	lines = append(lines, m.fit(st.Heading.Render(m.heading), w))
	if h > 1 {
		lines = append(lines, m.fit(st.Status.Render(strconv.Itoa(len(m.chosen))+" chosen"), w))
	}
	end := len(lines) + m.rows()
	if len(m.chosen) == 0 && end > len(lines) {
		lines = append(lines, m.fit(st.Empty.Render(m.emptyText), w))
	}
	for i := m.top; i < len(m.chosen) && len(lines) < end; i++ {
		lines = append(lines, m.fit(m.row(i), w))
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return m.frame().Render(strings.Join(lines, "\n"))
}

// row renders chosen item i: its place, its title and its direction.
func (m Model) row(i int) string {
	c := m.chosen[i]
	it := m.items[c.index]
	gutter, title := "  ", m.styles.Title
	if i == m.cursor && m.pane == PaneChosen {
		gutter, title = m.gutter, m.styles.SelectedTitle
	}
	label := it.AscLabel
	if c.desc {
		label = it.DescLabel
	}
	var b strings.Builder
	b.WriteString(gutter)
	b.WriteString(m.styles.Number.Render(strconv.Itoa(i+1) + ". "))
	b.WriteString(title.Render(it.Title))
	if label != "" {
		b.WriteString("  ")
		b.WriteString(m.styles.Direction.Render(label))
	}
	return b.String()
}

// fit cuts or pads s to w cells.
func (m Model) fit(s string, w int) string {
	s = termtext.Truncate(s, w, m.styles.Ellipsis)
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}
