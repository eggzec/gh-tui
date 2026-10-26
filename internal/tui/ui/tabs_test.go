package ui

import "testing"

func TestCycle(t *testing.T) {
	t.Parallel()
	c := Cycle[string]{"a", "b", "c"}
	for _, tc := range []struct {
		cur        string
		next, prev string
		index      int
	}{
		{"a", "b", "c", 0},
		{"b", "c", "a", 1},
		{"c", "a", "b", 2},
		{"?", "b", "c", 0},
	} {
		if got := c.Next(tc.cur); got != tc.next {
			t.Errorf("Next(%q) = %q, want %q", tc.cur, got, tc.next)
		}
		if got := c.Prev(tc.cur); got != tc.prev {
			t.Errorf("Prev(%q) = %q, want %q", tc.cur, got, tc.prev)
		}
		if got := c.Index(tc.cur); got != tc.index {
			t.Errorf("Index(%q) = %d, want %d", tc.cur, got, tc.index)
		}
	}
}
