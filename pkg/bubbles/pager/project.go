package pager

import "slices"

// The window scrolls through the lines shown, which are every line of the
// content unless vis picks some of them. Positions count the lines shown;
// line indices count the lines of the content, so the line numbers,
// matches and highlights keep to the content whatever is shown.

// count returns the number of lines shown.
func (m Model) count() int {
	if m.vis != nil {
		return len(m.vis)
	}
	return len(m.lines)
}

// at returns the index of the line shown at position p.
func (m Model) at(p int) int {
	if m.vis != nil {
		return int(m.vis[p])
	}
	return p
}

// posOf returns the position of line i if it is shown, or else of the
// first line shown after it, or count past the last.
func (m Model) posOf(i int) int {
	if m.vis == nil {
		return i
	}
	p, _ := slices.BinarySearch(m.vis, int32(i))
	return p
}

// topLine returns the index of the line at the top of the window, or 0
// when no line is shown.
func (m Model) topLine() int {
	if m.top >= m.count() {
		return 0
	}
	return m.at(m.top)
}
