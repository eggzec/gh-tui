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

// Fit cuts s like Truncate, then pads it with spaces on the right, so the
// result is exactly width cells wide. A width of no cells gives "".
func Fit(s string, width int, tail string) string {
	s = Truncate(s, width, tail)
	if w := ansi.StringWidth(s); w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

// TruncateMiddle cuts s to width cells like Truncate, but from the middle:
// its start and its end show, joined by tail where it is cut. A tail wider
// than width is left out, and one as wide as width shows alone. A wide rune
// that straddles a cut is dropped, so the result may be a cell short, but
// never wider than width.
func TruncateMiddle(s string, width int, tail string) string {
	if width <= 0 {
		return ""
	}
	w := ansi.StringWidth(s)
	if w <= width {
		return s
	}
	tw := ansi.StringWidth(tail)
	if tw > width {
		return ansi.Truncate(s, width, "")
	}
	keep := width - tw
	right := keep / 2
	// TruncateLeft keeps a wide rune that straddles its cut, so the end can be
	// a cell too wide; cut one cell further to drop that rune.
	end := ansi.TruncateLeft(s, w-right, "")
	if ansi.StringWidth(end) > right {
		end = ansi.TruncateLeft(s, w-right+1, "")
	}
	return ansi.Truncate(s, keep-right, "") + tail + end
}

// TruncateLeft cuts s to width cells from the left, so its end shows, and
// starts it with tail where it is cut. A tail as wide as width or wider is
// left out, so the end of s shows alone.
func TruncateLeft(s string, width int, tail string) string {
	if width <= 0 {
		return ""
	}
	w := ansi.StringWidth(s)
	if w <= width {
		return s
	}
	tw := ansi.StringWidth(tail)
	if tw >= width {
		return ansi.TruncateLeft(s, w-width, "")
	}
	return tail + ansi.TruncateLeft(s, w-width+tw, "")
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
