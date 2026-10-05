package ownerui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Frame draws body in a frame of w by h cells, with label, labelW cells
// wide and styled already, in its top edge, cut to end in tail when it
// doesn't fit. The edges are drawn with the lines of b, painted edge.
// Lines of body are padded or cut to the inside of the frame, and those
// past its height are left out.
func Frame(label string, labelW, w, h int, edge Paint, b lipgloss.Border, tail string, body []string) []string {
	if w < 2 || h < 2 {
		return nil
	}
	lines := make([]string, 0, h)
	if labelW > w-4 {
		label = termtext.Truncate(label, max(w-5, 0), tail)
		labelW = ansi.StringWidth(label)
	}
	if labelW == 0 {
		lines = append(lines, edge.Render(b.TopLeft+strings.Repeat(b.Top, w-2)+b.TopRight))
	} else {
		rest := max(w-4-labelW, 0)
		lines = append(lines, edge.Render(b.TopLeft+b.Top)+label+edge.Render(" "+strings.Repeat(b.Top, rest)+b.TopRight))
	}
	side, inner := edge.Render(b.Left), w-2
	for i := range h - 2 {
		var l string
		if i < len(body) {
			l = body[i]
		}
		lines = append(lines, side+Fit(l, inner)+side)
	}
	return append(lines, edge.Render(b.BottomLeft+strings.Repeat(b.Bottom, w-2)+b.BottomRight))
}

// Beside appends to lines those of left, each with the line of right
// beside it.
func Beside(lines, left, right []string) []string {
	for i, l := range left {
		if i < len(right) {
			l += right[i]
		}
		lines = append(lines, l)
	}
	return lines
}

// Indent puts a space before each of lines, as the lines of a pane start.
func Indent(lines []string) []string {
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	return lines
}
