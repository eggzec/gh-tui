// Package wrap walks plain text by grapheme and cell width. The pager and the
// log viewer share it to cut a long line into the window that shows, so
// walking a window costs the window, not the line.
package wrap

import (
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Advance walks the graphemes of the plain text s from byte i, as long as
// they fit in cols cells. It returns the byte where it stopped and the
// cells it walked. Unlike ansi.Truncate it stops there, so walking a window
// of a long line costs the window, not the line.
func Advance(s string, i, cols int) (j, used int) {
	for j = i; j < len(s); {
		// ASCII followed by ASCII is a grapheme of its own; the caller has
		// removed the control characters.
		if s[j] < utf8.RuneSelf && (j+1 == len(s) || s[j+1] < utf8.RuneSelf) {
			if used+1 > cols {
				return j, used
			}
			used++
			j++
			continue
		}
		g, w := ansi.FirstGraphemeCluster(s[j:], ansi.GraphemeWidth)
		if used+w > cols {
			return j, used
		}
		used += w
		j += len(g)
	}
	return j, used
}

// NextRow returns where the wrapped row of s that starts at byte i ends,
// and its width. A row holds at least one grapheme, so a grapheme wider
// than cols makes a row wider than cols.
func NextRow(s string, i, cols int) (j, used int) {
	j, used = Advance(s, i, cols)
	if j == i && i < len(s) {
		g, w := ansi.FirstGraphemeCluster(s[i:], ansi.GraphemeWidth)
		return i + len(g), w
	}
	return j, used
}

// LeftEdge returns the first byte of s shown when scrolled left columns
// sideways, and the blank cells before it where a wide grapheme is cut in
// half.
func LeftEdge(s string, left int) (a, pad int) {
	a, used := Advance(s, 0, left)
	if used < left && a < len(s) {
		e, w := Advance(s, a, 2)
		return e, used + w - left
	}
	return a, 0
}
