package filterform

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Least width of a dropdown, in cells, frame included.
const dropMinWidth = 24

// dropGutter is the width of the mark of the highlighted item that the
// picker draws before each item.
const dropGutter = 2

// dropTitle returns the title of the dropdown: the label of its row, and
// for a checklist how many of the options are checked.
func (m *Model) dropTitle() string {
	var label string
	switch {
	case m.tab == SortTab && m.row == sortByRow:
		label = m.spec.Sort.Label
	case m.tab == SortTab:
		label = orderLabel
	default:
		label = m.spec.Fields[m.row].Label
	}
	label = termtext.OneLine(label)
	if m.picking && m.kind() == Multi {
		label += m.styles.Glyphs.Separator + strconv.Itoa(len(m.state.values[m.row].list)) + " of " + strconv.Itoa(m.dropItems)
	}
	return label
}

// wantWidth returns the width a dropdown listing items wants, frame
// included: the longest item with its gutter, mark of markW cells and
// detail, the title, and the cells inside the frame that the text of its
// filter line and of an empty list need, and at least dropMinWidth.
func (m *Model) wantWidth(items []picker.Item, markW, inner int) int {
	w := max(ansi.StringWidth(m.dropTitle())+6, inner+2)
	for _, it := range items {
		iw := dropGutter + markW + 1 + ansi.StringWidth(termtext.OneLine(it.Title))
		if it.Detail != "" {
			iw += 2 + ansi.StringWidth(termtext.OneLine(it.Detail))
		}
		w = max(w, iw+2)
	}
	return max(w, dropMinWidth)
}

// dropHeight returns the height the dropdown wants, frame included: room
// for up to dropRows items, and the filter line when it has one. A
// dropdown that waits for its options or failed to load wants room for
// what it says.
func (m *Model) dropHeight() int {
	if !m.picking {
		return len(m.statusLines()) + 2
	}
	rows := m.dropItems
	if m.kind() == Person {
		// What the user types and finds is listed too, with the typed item.
		rows = max(rows+2, 1)
		if m.spec.Fields[m.row].Load != nil {
			rows = dropRows
		}
	}
	rows = min(max(rows, 1), dropRows)
	if m.pick.KeyMap().Normal.Insert.Enabled() {
		rows += 2
	}
	return rows + 2
}

// dropdown returns the box of the open dropdown and where it goes in the
// body of the form, which has bodyLines lines above the help, the first of
// the rows at index first and the row in focus at focusY. It opens under
// its row, or above it if it doesn't fit under and fits above, or on the
// side with more room, cut to fit. It returns "" if there is none, or no
// room for one.
func (m *Model) dropdown(bodyLines, first, focusY int) (box string, x, y int) {
	if m.mode != listMode || focusY < 0 {
		return "", 0, 0
	}
	x = m.editorX()
	want := m.dropWidth
	var status []string
	if !m.picking {
		status = m.statusLines()
		want = dropMinWidth
		for _, l := range status {
			want = max(want, ansi.StringWidth(l)+2)
		}
	}
	w, h := min(want, m.width-x), m.dropHeight()
	below, above := bodyLines-focusY-1, focusY-first
	switch {
	case below >= h:
		y = focusY + 1
	case above >= h:
		y = focusY - h
	case above > below:
		h, y = above, first
	default:
		h, y = below, focusY+1
	}
	if w < 3 || h < 3 {
		return "", 0, 0
	}
	var inner []string
	if m.picking {
		if m.pick.Width() != w-2 || m.pick.Height() != h-2 {
			m.pick.SetSize(w-2, h-2)
		}
		inner = strings.Split(m.pick.View(), "\n")
	} else {
		inner = make([]string, 0, h-2)
		for _, l := range status {
			inner = append(inner, fitCut(l, w-2, m.styles.ErrorEllipsis))
		}
	}
	return m.frameBox(inner, w, h), x, y
}

// frameBox draws lines, which are w-2 cells wide, in the frame of the
// dropdown, w by h cells, with its title in the top edge.
func (m *Model) frameBox(lines []string, w, h int) string {
	s := m.styles
	b := s.DropFrame.GetBorderStyle()
	edge := lipgloss.NewStyle().Foreground(s.DropFrame.GetBorderTopForeground())
	cell := func(g string) string { return edge.Render(termtext.Cells(g, 1)) }
	title := termtext.Truncate(m.dropTitle(), max(w-6, 0), s.Glyphs.Ellipsis)
	top := cell(b.TopLeft) + cell(b.Top) + " " + s.DropTitle.Render(title) + " "
	top += edge.Render(strings.Repeat(termtext.Cells(b.Top, 1), max(w-ansi.StringWidth(top)-1, 0))) + cell(b.TopRight)
	out := make([]string, 0, h)
	out = append(out, top)
	left, right := cell(b.Left), cell(b.Right)
	blank := strings.Repeat(" ", w-2)
	for i := range h - 2 {
		line := blank
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, left+line+right)
	}
	bottom := cell(b.BottomLeft) + edge.Render(strings.Repeat(termtext.Cells(b.Bottom, 1), w-2)) + cell(b.BottomRight)
	return strings.Join(append(out, bottom), "\n")
}
