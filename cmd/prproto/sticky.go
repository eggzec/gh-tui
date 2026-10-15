package main

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Sticky headers: a parent row that has scrolled above the top of a pane
// while its children are still in view stays pinned on the first lines.

const maxPins = 2

const pinBG = "\x1b[48;5;236m"

// pinRow marks a pinned row with a slightly dimmer background across the row.
func pinRow(l string, w int) string {
	l = strings.NewReplacer("\x1b[0m", "\x1b[0m"+pinBG, "\x1b[m", "\x1b[m"+pinBG).Replace(l)
	pad := strings.Repeat(" ", max(0, w-ansi.StringWidth(l)))
	return pinBG + l + pad + "\x1b[0m"
}

// depthAnc gives the ancestors of a line of a flat tree, outermost first: the
// nearest earlier lines with a smaller depth.
func depthAnc(depth []int) func(int) []int {
	return func(t int) []int {
		if t >= len(depth) {
			return nil
		}
		var out []int
		d := depth[t]
		for j := t - 1; j >= 0 && d > 0; j-- {
			if depth[j] < d {
				out = append(out, j)
				d = depth[j]
			}
		}
		slices.Reverse(out)
		return out
	}
}

// treeWindow picks the first content line of a pane ih rows high so that the
// cursor line cl stays below the pinned lines, which take rows of the pane.
func treeWindow(cl, ih int, anc func(int) []int) (int, []int) {
	maxK := min(maxPins, ih-1)
	for k := 0; ; k++ {
		off := max(0, cl-(ih-k)+1)
		p := anc(off)
		if len(p) <= k || k >= maxK {
			// The nearest ancestors win over the outermost ones.
			return off, p[max(0, len(p)-k):]
		}
	}
}

// paneDepths is the nesting of each line of a tree pane, nil for other panes.
func (m *model) paneDepths(i int) []int {
	var d []int
	switch i {
	case pReviewers:
		for _, n := range m.involved() {
			d = append(d, n.depth)
		}
	case pDev:
		for _, n := range m.devNodes() {
			d = append(d, n.depth)
		}
	case pFiles:
		for _, r := range m.rows {
			d = append(d, r.depth)
		}
	}
	return d
}

// convAnc gives the lines of the rows that are above line t but still hold
// it: the row it belongs to and that row's parents, outermost first.
func (c *convState) convAnc(t int) []int {
	if t < 0 || t >= len(c.lineRow) {
		return nil
	}
	var out []int
	for r := c.lineRow[t]; r >= 0; r = c.rowPar[r] {
		if c.rowFirst[r] < t {
			out = append(out, c.rowFirst[r])
		}
	}
	slices.Reverse(out)
	return out[max(0, len(out)-maxPins):]
}

// convPins are the lines pinned when the conversation's first content line
// is t.
func (m *model) convPins(t int) []int { return m.cv.convAnc(t) }

// scrollMax is the largest scroll of n lines in ih rows; the pinned lines take
// rows, so the last line needs a scroll a little past n-ih.
func (m *model) scrollMax(n, ih int) int {
	s := max(0, n-ih)
	for s < n-1 && s+ih-len(m.convPins(s)) < n {
		s++
	}
	return s
}
