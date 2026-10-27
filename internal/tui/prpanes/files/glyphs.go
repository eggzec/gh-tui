package files

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Glyphs are the marks of the rows, each one cell wide. A row follows each
// with a space, since the glyphs of a Nerd Font spill over into the next cell.
type Glyphs struct {
	// Commit marks a commit.
	Commit string
	// Pass, Fail and Pending mark how the checks of a commit stand.
	Pass, Fail, Pending string
	// Failure, Warning and Notice mark the worst level of the annotations the
	// checks left on a file.
	Failure, Warning, Notice string
	// Glyphs are the marks the panes share: Folded is what a closed fold
	// hides, and Dot goes between the parts of a title.
	prpanes.Glyphs
	// Rule draws the lines between the sections, and Cut ends a text that was
	// cut.
	Rule, Cut string
	// Minus goes before the lines a file deletes.
	Minus string
}

// newGlyphs returns the glyphs of the icon set named set, one of
// config.IconsNerd, config.IconsUnicode and config.IconsASCII, whose icons
// are ic.
func newGlyphs(set string, ic ui.Icons) Glyphs {
	g := Glyphs{
		Pass:    ic.Run(ui.RunSuccess),
		Fail:    ic.Run(ui.RunFailure),
		Pending: ic.Run(ui.RunInProgress),
		Failure: ic.No,
		Warning: ic.Warning,
		Notice:  ic.Info,
		Glyphs:  prpanes.GlyphsFrom(ic),
		Rule:    ic.Border.Top,
		Cut:     ic.Ellipsis,
		Minus:   ic.Minus,
	}
	switch set {
	case config.IconsUnicode:
		g.Commit = "●"
	case config.IconsASCII:
		g.Commit = "o"
	default:
		// oct-git_commit.
		g.Commit = ""
	}
	return g
}
