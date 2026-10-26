package ui

import "slices"

// Cycle is the tabs of a bar in the order the next and previous keys go
// through them, round from the last to the first.
type Cycle[T comparable] []T

// Index returns the place of cur, or 0 when it is not one of the tabs.
func (c Cycle[T]) Index(cur T) int {
	return max(slices.Index(c, cur), 0)
}

// Next returns the tab after cur.
func (c Cycle[T]) Next(cur T) T {
	return c[(c.Index(cur)+1)%len(c)]
}

// Prev returns the tab before cur.
func (c Cycle[T]) Prev(cur T) T {
	return c[(c.Index(cur)+len(c)-1)%len(c)]
}
