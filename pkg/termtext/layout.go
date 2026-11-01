package termtext

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Spread puts left and right on a line of w cells, with right against the
// edge. Left is cut to leave right whole, while right fits, and ends in
// tail, an ellipsis, where it is cut. The line is exactly w cells wide.
func Spread(left, right string, w int, tail string) string {
	rw := ansi.StringWidth(right)
	if rw == 0 || rw+2 > w {
		return Fit(left, w, "")
	}
	left = Truncate(left, w-rw-1, tail)
	return left + strings.Repeat(" ", w-ansi.StringWidth(left)-rw) + right
}

// PadLines returns exactly h of lines, which are w cells wide already: the
// lines past h are left out, and blank ones fill the rest.
func PadLines(lines []string, w, h int) []string {
	h = max(h, 0)
	if len(lines) >= h {
		return lines[:h]
	}
	blank := strings.Repeat(" ", max(w, 0))
	for len(lines) < h {
		lines = append(lines, blank)
	}
	return lines
}
