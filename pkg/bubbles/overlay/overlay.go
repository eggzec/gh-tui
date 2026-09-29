// Package overlay draws a box, such as a modal, over a rendered frame.
//
// Each row the box covers is cut around it: what the frame shows left of
// the box, then the box's row, then what the frame shows right of it, so
// wide runes and styles survive on both sides. The rows above and below
// the box are copied as they are. It draws the same cells as the
// cell-based compositor of lipgloss, without parsing the rows into cells,
// which is most of the cost of a frame with a modal open.
package overlay

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// reset ends the styles of what comes before it, so that the box and the
// frame right of it don't take on the styles of what is left of them.
const reset = "\x1b[m"

// Place draws fg over bg with its top-left corner at column x and row y of
// bg, and returns the result at the size of bg. Parts of fg outside bg are
// cut off, and negative coordinates count as 0. The rows of bg that fg
// covers set the width it is cut at, and only they are measured.
func Place(bg, fg string, x, y int) string {
	x, y = max(x, 0), max(y, 0)
	rows := strings.Split(bg, "\n")
	if y >= len(rows) {
		return bg
	}
	box := strings.Split(fg, "\n")
	end := min(y+len(box), len(rows))
	widths := make([]int, end-y)
	width := 0
	for i := range widths {
		widths[i] = ansi.StringWidth(rows[y+i])
		width = max(width, widths[i])
	}
	if x >= width {
		return bg
	}
	var b strings.Builder
	b.Grow(len(bg) + len(fg) + len(widths)*2*len(reset))
	for i, r := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i < y || i >= end {
			b.WriteString(r)
			continue
		}
		cover(&b, r, widths[i-y], box[i-y], x, width)
	}
	return b.String()
}

// cover writes row, which is rw cells wide, with line, a row of the box,
// over it from column x, cut at width.
func cover(b *strings.Builder, row string, rw int, line string, x, width int) {
	lw := ansi.StringWidth(line)
	if lw > width-x {
		line, lw = cut(line, width-x), width-x
	}
	left, right := split(row, x, x+lw)
	b.WriteString(left)
	endStyles(b, left)
	if rw < x {
		b.WriteString(strings.Repeat(" ", x-rw))
	}
	b.WriteString(line)
	endStyles(b, line)
	if rw > x+lw {
		b.WriteString(right)
	}
}

// split returns what row shows left of column x, and right of column r,
// in one pass. The right part starts with every escape sequence before it,
// so it keeps the styles and link it starts in. A wide rune that either
// column splits gives way to a space in its styles, as the cells of a
// terminal would.
func split(row string, x, r int) (left, right string) {
	var (
		state byte
		col   int
		seqs  strings.Builder
		open  = true // still left of the box
	)
	for i := 0; i < len(row); {
		seq, w, n, next := ansi.DecodeSequence(row[i:], state, nil)
		state = next
		if w == 0 {
			seqs.WriteString(seq)
			i += n
			continue
		}
		if open && col+w > x {
			open = false
			left = row[:i]
			if col < x {
				left += strings.Repeat(" ", x-col)
			}
		}
		switch {
		case open:
		case col >= r:
			return left, seqs.String() + row[i:]
		case col+w > r:
			return left, seqs.String() + strings.Repeat(" ", col+w-r) + row[i+n:]
		}
		col += w
		i += n
	}
	if open {
		left = row
	}
	return left, ""
}

// endStyles ends the styles and the link of s, which b ends with, unless
// s does, so what follows doesn't take them on.
func endStyles(b *strings.Builder, s string) {
	if !strings.Contains(s, "\x1b") {
		return
	}
	if !strings.HasSuffix(s, reset) {
		b.WriteString(reset)
	}
	if linkOpen(s) {
		b.WriteString(ansi.ResetHyperlink())
	}
}

// linkOpen reports whether s ends inside a hyperlink: whether its last
// OSC 8 sequence opens one, with a URL, rather than closes it.
func linkOpen(s string) bool {
	i := strings.LastIndex(s, "\x1b]8;")
	if i < 0 {
		return false
	}
	_, rest, ok := strings.Cut(s[i+len("\x1b]8;"):], ";")
	return ok && rest != "" && rest[0] != '\x1b' && rest[0] != '\a'
}

// cut truncates s to n cells. A wide rune that the cut splits gives way
// to a space in its styles, which Truncate puts where it cut.
func cut(s string, n int) string {
	t := ansi.Truncate(s, n, "")
	if ansi.StringWidth(t) < n {
		t = ansi.Truncate(s, n, " ")
	}
	return t
}

// Center draws fg over the middle of bg, which is width cells wide and
// height rows high, such as a root view at the size of the terminal. A box
// larger than bg is aligned to its top-left corner and cut off.
func Center(bg, fg string, width, height int) string {
	x := (width - lipgloss.Width(fg)) / 2
	y := (height - lipgloss.Height(fg)) / 2
	return Place(bg, fg, x, y)
}
