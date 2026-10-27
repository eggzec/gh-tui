// Package prpanes holds what the panes of the pull request modal share.
package prpanes

import "github.com/eggzec/gh-tui/internal/tui/ui"

// Glyphs are the marks the panes draw from the icon set the same way, so a
// pane embeds them and keeps only the marks of its own.
type Glyphs struct {
	// Separator goes between the parts of the right side of a row, and
	// Ellipsis ends a text that was cut.
	Separator, Ellipsis string
	// Dot goes between the parts of a head, and Dash between two ages.
	Dot, Dash string
	// Folded stands for what a closed fold hides.
	Folded string
}

// GlyphsFrom returns the shared glyphs of the icon set ic.
func GlyphsFrom(ic ui.Icons) Glyphs {
	return Glyphs{
		Separator: ic.Separator,
		Ellipsis:  ic.Ellipsis,
		Dot:       ic.Dot,
		Dash:      ic.Dash,
		Folded:    ic.Folded,
	}
}
