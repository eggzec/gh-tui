package ui

import (
	"strconv"
	"strings"
)

// Count formats n the way GitHub shows counts: 999, 1.2k, 12k. more adds a
// "+" for a list that is not fully loaded, so n is only a lower bound.
func Count(n int, more bool) string {
	s := count(n)
	if more {
		s += "+"
	}
	return s
}

func count(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 10_000:
		s := strconv.FormatFloat(float64(n)/1000, 'f', 1, 64)
		return strings.TrimSuffix(s, ".0") + "k"
	case n < 1_000_000:
		return strconv.Itoa(n/1000) + "k"
	}
	return strconv.Itoa(n/1_000_000) + "m"
}
