package checktree

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Glyphs are the marks of the rows, each one cell wide. A row follows each
// with a space, since the glyphs of a Nerd Font spill over into the next cell.
type Glyphs struct {
	// Pass, Fail and Pending mark how a check stands, as the Actions modal
	// marks its runs.
	Pass, Fail, Pending string
	// Actions, App and Status mark the kind of a group of checks, and of a
	// commit status.
	Actions, App, Status string
	// Failure, Warning and Notice mark the worst level of the annotations of
	// a check run.
	Failure, Warning, Notice string
	// Glyphs are the marks the panes share: Separator goes between the parts
	// of the right side of a row, and Ellipsis ends a text that was cut.
	prpanes.Glyphs
}

// newGlyphs returns the glyphs of the icon set named set, one of
// config.IconsNerd, config.IconsUnicode and config.IconsASCII.
func newGlyphs(set string, ic ui.Icons) Glyphs {
	g := Glyphs{
		Pass:    ic.Run(ui.RunSuccess),
		Fail:    ic.Run(ui.RunFailure),
		Pending: ic.Run(ui.RunInProgress),
		Failure: ic.No,
		Warning: ic.Warning,
		Notice:  ic.Info,
		Glyphs:  prpanes.GlyphsFrom(ic),
	}
	switch set {
	case config.IconsUnicode:
		g.Actions, g.App, g.Status = "▶", "▣", "•"
	case config.IconsASCII:
		g.Actions, g.App, g.Status = ">", "#", "."
	default:
		// oct-play, oct-apps and oct-dot_fill.
		g.Actions, g.App, g.Status = "", "", ""
	}
	return g
}
