package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// SpreadDrop puts left and right at the edges of width cells, and drops right
// when both don't fit, cutting left to end in tail when it doesn't.
func SpreadDrop(left, right string, width int, tail string) string {
	lw, rw := ansi.StringWidth(left), ansi.StringWidth(right)
	if right == "" || lw+rw+1 > width {
		return Fit(termtext.Truncate(left, width, tail), width)
	}
	return left + strings.Repeat(" ", width-lw-rw) + right
}

// CleanLine puts text from GitHub on one line without escape sequences or
// controls, so it can't break the layout.
func CleanLine(s string) string {
	return strings.Join(strings.Fields(OneLine(s)), " ")
}

// Count formats n the way GitHub shows counts: 999, 1.2k, 12k.
func Count(n int) string {
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

// Plural returns one when n is 1 and many otherwise.
func Plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
