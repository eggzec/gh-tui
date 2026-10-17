package files

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Glyphs are the marks of the rows, each one cell wide. A row follows each
// with a space, since the glyphs of a Nerd Font spill over into the next cell.
type Glyphs struct {
	// Commit marks a commit.
	Commit string
	// Pass, Fail and Pending mark how the checks of a commit stand.
	Pass, Fail, Pending string
	// Fold is what a closed fold hides, Dot goes between the parts of a
	// title, Rule draws the lines between the sections, and Cut ends a text
	// that was cut.
	Fold, Dot, Rule, Cut string
	// Minus goes before the lines a file deletes.
	Minus string
}

// NewGlyphs returns the glyphs of the icon set named set, one of
// config.IconsNerd, config.IconsUnicode and config.IconsASCII, whose icons
// are ic.
func NewGlyphs(set string, ic ui.Icons) Glyphs {
	g := Glyphs{
		Pass:    ic.Run(ui.RunSuccess),
		Fail:    ic.Run(ui.RunFailure),
		Pending: ic.Run(ui.RunInProgress),
		Cut:     ic.Ellipsis,
		Minus:   ic.Minus,
	}
	switch set {
	case config.IconsUnicode:
		g.Commit, g.Fold, g.Dot, g.Rule = "●", "⋯", "·", "─"
	case config.IconsASCII:
		g.Commit, g.Fold, g.Dot, g.Rule = "o", "...", "-", "-"
	default:
		// oct-git_commit.
		g.Commit, g.Fold, g.Dot, g.Rule = "", "⋯", "·", "─"
	}
	return g
}
