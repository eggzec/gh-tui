package ui

import (
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

// Plural returns one when n is 1 and many otherwise.
func Plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
