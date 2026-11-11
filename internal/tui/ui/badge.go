package ui

import "charm.land/lipgloss/v2"

// Badge is text set off as a label: bold and reversed in style, with a space
// on each side, and icon before the text when it has one.
func Badge(style lipgloss.Style, icon, text string) string {
	if icon != "" {
		text = icon + " " + text
	}
	return style.Bold(true).Reverse(true).Render(" " + text + " ")
}
