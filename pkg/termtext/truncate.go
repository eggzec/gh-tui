package termtext

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Truncate cuts s, which may hold escape sequences, to width cells, and
// ends it with tail where it is cut, as ansi.Truncate does. A tail wider
// than width is left out, so a cut too narrow for an ellipsis such as
// "..." still shows the start of s rather than nothing. A cut to no cells
// is empty, without the escape sequences of s.
func Truncate(s string, width int, tail string) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(tail) > width {
		tail = ""
	}
	return ansi.Truncate(s, width, tail)
}

// Cells cuts or pads g, a glyph such as a cursor or a mark, to exactly n
// cells, so the columns after it stay where they are whatever glyph a
// program picks.
func Cells(g string, n int) string {
	g = ansi.Truncate(g, n, "")
	if w := ansi.StringWidth(g); w < n {
		g += strings.Repeat(" ", n-w)
	}
	return g
}
