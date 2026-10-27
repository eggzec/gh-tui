package ui

import "strings"

// Rule returns a horizontal rule w cells wide, drawn with the top border of
// icons. A width below zero gives an empty rule. The caller styles it.
func Rule(icons Icons, w int) string {
	return strings.Repeat(icons.Border.Top, max(w, 0))
}
