package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Column is one column of a list's header: its title and the width its rows
// give it. A width of 0 or less drops the column, as it drops the cell in
// the rows.
type Column struct {
	Title string
	Width int
	// Right aligns the title to the right edge of the column, as the rows
	// align their numbers. Everything else is left-aligned.
	Right bool
}

// Header renders the line of titles above a list's rows, each in the width
// of its column and gap cells apart. A title wider than its column ends in
// tail. The line is plain text; the list styles it.
func Header(cols []Column, gap int, tail string) string {
	var b strings.Builder
	for _, c := range cols {
		if c.Width <= 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(strings.Repeat(" ", gap))
		}
		text := termtext.Truncate(c.Title, c.Width, tail)
		pad := strings.Repeat(" ", max(c.Width-ansi.StringWidth(text), 0))
		if c.Right {
			b.WriteString(pad)
			b.WriteString(text)
		} else {
			b.WriteString(text)
			b.WriteString(pad)
		}
	}
	return b.String()
}
