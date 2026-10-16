package conv

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Glyphs are the marks of the rows that the icon sets of [ui] don't hold. The
// glyphs of reviews and threads are those of [ui.GlyphStyles], one cell
// wide, and a row follows each with a space since the glyphs of a Nerd Font
// spill over into the next cell.
type Glyphs struct {
	// Fold stands for what a closed fold hides, and Cut ends a text that was
	// cut.
	Fold, Cut string
	// Dot goes between the parts of a head, and Dash between the two ages of
	// a run.
	Dot, Dash string
}

// NewGlyphs returns the glyphs of the icon set named set, one of
// config.IconsNerd, config.IconsUnicode and config.IconsASCII, whose icons
// are ic.
func NewGlyphs(set string, ic ui.Icons) Glyphs {
	if set == config.IconsASCII {
		return Glyphs{Fold: "...", Cut: ic.Ellipsis, Dot: "-", Dash: "-"}
	}
	return Glyphs{Fold: "⋯", Cut: ic.Ellipsis, Dot: "·", Dash: "–"}
}
