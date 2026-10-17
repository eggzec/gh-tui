package merge

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// Styles are what the rows and lines are drawn with. Build them once, with
// [NewStyles], and again when the theme or the icons change.
type Styles struct {
	// Checks draws the rows of the checks.
	Checks checktree.Styles
	// Ready is the glyph that marks a pull request with no blocker, in the
	// color of success.
	Ready string

	glyphs   ui.GlyphStyles
	dim      lipgloss.Style
	bold     lipgloss.Style
	arrow    string
	sep      string
	top      string
	ellipsis string
}

// NewStyles returns the styles for theme t and the icon set named set, whose
// icons are ic.
func NewStyles(t ui.Theme, ic ui.Icons, set string) Styles {
	ck := checktree.NewStyles(t, ic, set)
	top := ic.Border.Top
	if top == "" {
		top = "-"
	}
	return Styles{
		Checks:   ck,
		Ready:    t.Success.Render(ck.Glyphs.Pass),
		glyphs:   ui.NewGlyphStyles(t, ic),
		dim:      t.Subtle,
		bold:     lipgloss.NewStyle().Bold(true),
		arrow:    t.Subtle.Render(" " + ic.Arrow + " "),
		sep:      t.Subtle.Render(ic.Separator),
		top:      top,
		ellipsis: ic.Ellipsis,
	}
}

// row returns the row of a blocker, which enter does something of its own on.
func (s *Styles) row(id string, g ui.Glyph, line string) foldtree.Row {
	return foldtree.Row{ID: id, Lines: []string{s.glyphs.Render(g) + " " + line}, Action: true}
}
