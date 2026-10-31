package statusbar

import "charm.land/lipgloss/v2"

// Hint returns the item of one key hint: key in keyStyle, a space, and desc
// in descStyle, with the given rank. It has one form.
func Hint(key, desc string, keyStyle, descStyle lipgloss.Style, rank int) Item {
	return Item{Forms: []string{keyStyle.Render(key) + " " + descStyle.Render(desc)}, Rank: rank}
}

// Pair returns a label and its value as the bar tells them: the label in
// labelStyle with a space after it, then the value in valueStyle.
func Pair(label, value string, labelStyle, valueStyle lipgloss.Style) string {
	return labelStyle.Render(label+" ") + valueStyle.Render(value)
}
