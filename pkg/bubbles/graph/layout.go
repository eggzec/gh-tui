package graph

import "charm.land/lipgloss/v2"

// Sides of a graph slot that a line touches. A slot's glyph follows from the
// sides it joins, the way box-drawing characters do.
const (
	up uint8 = 1 << iota
	down
	left
	right
)

// Glyph IDs index the rendered fragments. Line glyphs follow glyphLine at the
// offset of their sides.
const (
	glyphSpace uint8 = iota
	glyphCommit
	glyphOverflow
	glyphLine
	glyphCount = glyphLine + (up | down | left | right) + 1
)

// glyphs returns the characters of the glyph IDs: commit marks a commit,
// overflow the lanes not drawn, and lines draws the lanes.
func glyphs(commit, overflow string, lines lipgloss.Border) [glyphCount]string {
	return [glyphCount]string{
		glyphSpace:    " ",
		glyphCommit:   commit,
		glyphOverflow: overflow,

		glyphLine:                              " ",
		glyphLine + up:                         lines.Left,
		glyphLine + down:                       lines.Left,
		glyphLine + (up | down):                lines.Left,
		glyphLine + left:                       lines.Top,
		glyphLine + right:                      lines.Top,
		glyphLine + (left | right):             lines.Top,
		glyphLine + (up | left):                lines.BottomRight,
		glyphLine + (up | right):               lines.BottomLeft,
		glyphLine + (down | left):              lines.TopRight,
		glyphLine + (down | right):             lines.TopLeft,
		glyphLine + (up | down | left):         lines.MiddleRight,
		glyphLine + (up | down | right):        lines.MiddleLeft,
		glyphLine + (up | left | right):        lines.MiddleBottom,
		glyphLine + (down | left | right):      lines.MiddleTop,
		glyphLine + (up | down | left | right): lines.Middle,
	}
}

// cell is one slot of a row of the graph: a glyph, and after it the gap to
// the next slot, which is blank or part of a horizontal line.
type cell struct {
	glyph uint8
	// color is the lane whose color the glyph takes.
	color uint8
	// gap is 0 for a blank gap, or 1 plus the lane whose color the line in
	// the gap takes.
	gap uint8
}

// layout assigns commits to lanes one row at a time, the way git log --graph
// does, so a chunk only continues the layout of the chunks before it.
//
// Each lane waits for one commit: the parent of the commit drawn last in it.
// A commit takes the leftmost lane that waits for it, or a free one, and the
// other lanes waiting for it end there. Its first parent continues its lane,
// and every other parent joins a lane that already waits for it, or opens a
// new one.
type layout struct {
	// lanes[i] is the ID lane i waits for, or "" when it is free.
	lanes []string
	// maxLanes is the number of lanes drawn; later ones fold into one
	// overflow slot.
	maxLanes int

	// Scratch space for one row, kept to save allocations.
	sides []uint8
	// hcolor[i] is the lane of the horizontal line through slot i, and
	// gaps[i] the lane of the line in the gap after it, plus one.
	hcolor []int
	gaps   []int
}

func newLayout(maxLanes int) layout {
	return layout{maxLanes: max(maxLanes, 1)}
}

// add lays out c, whose parents are those not seen yet, and appends its row
// to dst.
func (l *layout) add(c Commit, seen func(id string) bool, dst []cell) []cell {
	n := len(l.lanes)
	width := n + len(c.Parents) + 1
	l.sides = resize(l.sides, width)
	l.hcolor = resize(l.hcolor, width)
	l.gaps = resize(l.gaps, width)
	for i, id := range l.lanes {
		if id != "" {
			l.sides[i] |= up
		}
	}

	col := index(l.lanes, c.ID, 0)
	if col < 0 {
		col = l.free(-1)
	} else {
		// The other lanes waiting for c end here.
		for j := col + 1; j < n; j++ {
			if l.lanes[j] == c.ID {
				l.lanes[j] = ""
				l.join(col, j)
			}
		}
	}

	l.lanes[col] = ""
	first := true
	for i, p := range c.Parents {
		// A parent shown already, above its child, has nowhere to draw.
		if p == "" || seen(p) || index(c.Parents[:i], p, 0) >= 0 {
			continue
		}
		if first {
			first = false
			l.lanes[col] = p
			continue
		}
		k := index(l.lanes, p, 0)
		if k < 0 {
			k = l.free(col)
			l.lanes[k] = p
		}
		l.join(col, k)
	}

	for i, id := range l.lanes {
		if id != "" {
			l.sides[i] |= down
		}
	}
	for len(l.lanes) > 0 && l.lanes[len(l.lanes)-1] == "" {
		l.lanes = l.lanes[:len(l.lanes)-1]
	}
	return l.cells(col, dst)
}

// free returns a free lane for a commit in lane col, preferring the right of
// col, and opens a new one if there is none. A col of -1 prefers the left.
func (l *layout) free(col int) int {
	if k := index(l.lanes, "", col+1); k >= 0 {
		return k
	}
	if k := index(l.lanes[:max(min(col, len(l.lanes)), 0)], "", 0); k >= 0 {
		return k
	}
	l.lanes = append(l.lanes, "")
	return len(l.lanes) - 1
}

// join draws a horizontal line from the commit in lane col to lane k, in
// the color of lane k.
func (l *layout) join(col, k int) {
	lo, hi := min(col, k), max(col, k)
	l.sides[lo] |= right
	l.sides[hi] |= left
	for i := lo; i < hi; i++ {
		if i > lo {
			l.sides[i] |= left | right
			l.hcolor[i] = k
		}
		l.gaps[i] = k + 1
	}
}

// cells appends the row of a commit in lane col to dst, folding the lanes
// past maxLanes into one overflow slot, and clears the scratch space.
func (l *layout) cells(col int, dst []cell) []cell {
	width := col + 1
	for i, s := range l.sides {
		if s != 0 {
			width = max(width, i+1)
		}
	}
	shown := min(width, l.maxLanes)
	for i := range shown {
		c := cell{glyph: glyphLine + l.sides[i], color: uint8(i), gap: uint8(l.gaps[i])}
		switch {
		case i == col:
			c.glyph = glyphCommit
		case l.sides[i]&(up|down) == 0 && l.sides[i] != 0:
			// A line that only passes through takes the color of the
			// lane it leads to.
			c.color = uint8(l.hcolor[i])
		}
		if i == width-1 {
			c.gap = 0
		}
		dst = append(dst, c)
	}
	if width > shown {
		c := cell{glyph: glyphOverflow}
		if col >= shown {
			c = cell{glyph: glyphCommit, color: uint8(col)}
		}
		dst = append(dst, c)
	}
	clear(l.sides)
	clear(l.hcolor)
	clear(l.gaps)
	return dst
}

// index returns the index of the first id in s at or after from, or -1.
func index(s []string, id string, from int) int {
	for i := from; i < len(s); i++ {
		if s[i] == id {
			return i
		}
	}
	return -1
}

// resize returns s with length n and every element zero.
func resize[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	s = s[:n]
	clear(s)
	return s
}
