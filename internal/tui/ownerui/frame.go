package ownerui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Digits is the strip of pane keys for the bottom edge of a frame, like the
// pane strip of the pull request modal: the keys in order, the one of pane
// cur in brackets and painted on, the others off. A pane with no key ("")
// is left out, and when pane cur has no key, no key is in brackets. It returns the strip and its width, or "" when no pane has a
// key.
func Digits(keys []string, cur int, on, off Paint) (strip string, width int) {
	return strip1(keys, nil, cur, on, off)
}

// Foot is the strip of pane keys with the title of each pane after its key,
// "1 Pinned  [2] Repositories  3 Waiting on you", for the bottom edge of a
// zoomed pane. It tries each of tiers, a list of titles by pane, from the
// first, and keeps the first strip no wider than maxW; when none fits, it is
// the keys alone, as Digits makes them.
func Foot(keys []string, tiers [][]string, cur int, on, off Paint, maxW int) (strip string, width int) {
	for _, titles := range tiers {
		if s, w := strip1(keys, titles, cur, on, off); w <= maxW {
			return s, w
		}
	}
	return Digits(keys, cur, on, off)
}

// strip1 writes one strip: each key, with the title of its pane after it
// when titles has one, the one of pane cur in brackets.
func strip1(keys, titles []string, cur int, on, off Paint) (strip string, width int) {
	var b strings.Builder
	n := 0
	sep := " "
	if titles != nil {
		sep = "  "
	}
	for i, k := range keys {
		if k == "" {
			continue
		}
		if n > 0 {
			b.WriteString(sep)
			n += len(sep)
		}
		if i == cur {
			k = "[" + k + "]"
		}
		if i < len(titles) && titles[i] != "" {
			k += " " + titles[i]
		}
		if i == cur {
			on.Write(&b, k)
		} else {
			off.Write(&b, k)
		}
		n += ansi.StringWidth(k)
	}
	return b.String(), n
}

// Frame draws body in a frame of w by h cells, with label, labelW cells
// wide and styled already, in its top edge, cut to end in tail when it
// doesn't fit, and foot, footW cells wide and styled already, in its bottom
// edge, left out when it doesn't fit. The edges are drawn with the lines of
// b, painted edge. Lines of body are padded or cut to the inside of the
// frame, and those past its height are left out.
func Frame(label string, labelW int, foot string, footW, w, h int, edge Paint, b lipgloss.Border, tail string, body []string) []string {
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
	if footW == 0 || footW > w-5 {
		return append(lines, edge.Render(b.BottomLeft+strings.Repeat(b.Bottom, w-2)+b.BottomRight))
	}
	rest := w - 5 - footW
	return append(lines, edge.Render(b.BottomLeft+b.Bottom+" ")+foot+edge.Render(" "+strings.Repeat(b.Bottom, rest)+b.BottomRight))
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
