package panes

// MinHeight is the fewest lines a pane of a stack keeps when the user grows
// its neighbour.
const MinHeight = 3

// Grow says which divider of a stack of n panes moves the focused pane, and the
// sign that makes it grow: a pane takes its rows from the pane below it, or from
// the one above when it is the last. Divider i lies between pane i and pane
// i+1, and a positive move pushes it down.
func Grow(focus, n int) (div, sign int) {
	if focus < n-1 {
		return focus, 1
	}
	return focus - 1, -1
}
