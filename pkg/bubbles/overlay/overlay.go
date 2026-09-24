// Package overlay draws a box, such as a modal, over a rendered frame.
//
// It is a thin helper over the cell-based compositor of lipgloss: the rows
// the box covers are parsed into cells, so wide runes and styles survive,
// and the cells of the frame on either side of the box keep their styles.
// The rows above and below the box are copied as they are.
package overlay

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Place draws fg over bg with its top-left corner at column x and row y of
// bg, and returns the result at the size of bg. Parts of fg outside bg are
// cut off, and negative coordinates count as 0. Only the rows of bg that fg
// covers are measured and parsed. Like the lipgloss canvas it
// trims plain spaces at the ends of the rows it draws on, which look the
// same.
func Place(bg, fg string, x, y int) string {
	x, y = max(x, 0), max(y, 0)
	rows := strings.Split(bg, "\n")
	if y >= len(rows) {
		return bg
	}
	end := min(y+lipgloss.Height(fg), len(rows))
	covered := strings.Join(rows[y:end], "\n")
	// The covered rows set the width, so the box is cut at their edge.
	canvas := lipgloss.NewCanvas(lipgloss.Width(covered), end-y)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(covered),
		lipgloss.NewLayer(fg).X(x).Z(1),
	))

	var b strings.Builder
	b.Grow(len(bg) + len(fg))
	for _, r := range rows[:y] {
		b.WriteString(r)
		b.WriteByte('\n')
	}
	b.WriteString(canvas.Render())
	for _, r := range rows[end:] {
		b.WriteByte('\n')
		b.WriteString(r)
	}
	return b.String()
}

// Center draws fg over the middle of bg, which is width cells wide and
// height rows high, such as a root view at the size of the terminal. A box
// larger than bg is aligned to its top-left corner and cut off.
func Center(bg, fg string, width, height int) string {
	x := (width - lipgloss.Width(fg)) / 2
	y := (height - lipgloss.Height(fg)) / 2
	return Place(bg, fg, x, y)
}
