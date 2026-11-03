package ui

import "github.com/eggzec/gh-tui/internal/core"

// LabelStripRows is the rows a modal gives the strip of the labels of what it
// shows: one if it has any, else none, so the panes have the row.
func LabelStripRows(labels []core.Label) int {
	if len(labels) > 0 {
		return 1
	}
	return 0
}

// LabelStrip is the line of the labels as far as they fit in width, and the
// number of the others, drawn with chips. more styles that number.
func LabelStrip(labels []core.Label, chips LabelChips, width int, more func(string) string) string {
	text := make([]string, len(labels))
	widths := make([]int, len(labels))
	for i, l := range labels {
		text[i], widths[i] = chips.Get(l)
	}
	return ChipRow(text, widths, width, more)
}
