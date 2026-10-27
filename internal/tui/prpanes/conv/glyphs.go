package conv

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Glyphs are the marks of the rows that the icon sets of [ui] don't hold. The
// glyphs of reviews and threads are those of [ui.GlyphStyles], one cell
// wide, and a row follows each with a space since the glyphs of a Nerd Font
// spill over into the next cell.
type Glyphs struct {
	// Glyphs are the marks the panes share: Folded stands for what a closed
	// fold hides, Dot goes between the parts of a head, and Dash between the
	// two ages of a run.
	prpanes.Glyphs
	// Cut ends a text that was cut.
	Cut string
	// Bar runs beside the content of an open details block, and Corner ends
	// it. Both are two cells wide.
	Bar, Corner string
}

// newGlyphs returns the glyphs of the icon set named set, one of
// config.IconsNerd, config.IconsUnicode and config.IconsASCII, whose icons
// are ic.
func newGlyphs(set string, ic ui.Icons) Glyphs {
	g := Glyphs{Glyphs: prpanes.GlyphsFrom(ic), Cut: ic.Ellipsis, Bar: "│ ", Corner: "╰─"}
	if set == config.IconsASCII {
		g.Bar, g.Corner = "| ", "`-"
	}
	return g
}
