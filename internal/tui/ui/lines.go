package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The helpers below lay out styled text in cells, for views such as the
// panes of a modal that render exactly their size.

// Fit pads or cuts s to w cells.
func Fit(s string, w int) string {
	sw := ansi.StringWidth(s)
	switch {
	case sw == w:
		return s
	case sw < w:
		return s + strings.Repeat(" ", w-sw)
	}
	return ansi.Truncate(s, w, "")
}

// FitLines returns exactly h lines of exactly w cells: lines cut or padded.
func FitLines(lines []string, w, h int) []string {
	out := make([]string, max(h, 0))
	for i := range out {
		if i < len(lines) {
			out[i] = Fit(lines[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

// Wrap wraps s to lines of w cells.
func Wrap(s string, w int) []string {
	lines := strings.Split(ansi.Wrap(s, max(w, 1), ""), "\n")
	for i, l := range lines {
		lines[i] = Fit(l, w)
	}
	return lines
}

// FirstLine cuts s at its first line.
func FirstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}

// OneLine joins the lines of s, and drops the escape sequences, the
// control characters and the invisible format characters, as
// [termtext.OneLine] does.
func OneLine(s string) string {
	return termtext.OneLine(s)
}
