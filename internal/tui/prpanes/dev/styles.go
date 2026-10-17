package dev

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// Glyphs are the marks that the rows draw besides the state of an item.
type Glyphs struct {
	// Blocked marks what stands in the way of an issue.
	Blocked string
	// Separator goes between the parts of a line, and Ellipsis ends a text
	// that was cut.
	Separator, Ellipsis string
}

// NewGlyphs returns the glyphs of the icon set ic.
func NewGlyphs(ic ui.Icons) Glyphs {
	return Glyphs{Blocked: ic.Glyph(ui.GlyphBlocked), Separator: ic.Separator, Ellipsis: ic.Ellipsis}
}

// Styles are what the rows are drawn with. Build them once, with
// [NewStyles], and again when the theme or the icons change.
type Styles struct {
	Glyphs Glyphs
	// Tree styles the tree that holds the rows.
	Tree foldtree.Styles

	icons  ui.Icons
	states [ui.NumStates]lipgloss.Style
	bold   lipgloss.Style
	dim    lipgloss.Style
	warn   lipgloss.Style
}

// NewStyles returns the styles for theme t and the icons ic.
func NewStyles(t ui.Theme, ic ui.Icons) Styles {
	s := Styles{
		Glyphs: NewGlyphs(ic),
		Tree:   foldtree.DefaultStyles(t.Dark),
		icons:  ic,
		bold:   t.Text.Bold(true),
		dim:    t.Subtle,
		warn:   t.Warning,
	}
	for i := range s.states {
		s.states[i] = t.State(ui.State(i))
	}
	return s
}

// state draws the glyph of state st in its colour.
func (s Styles) state(st ui.State) string {
	return s.states[st].Render(s.icons.State(st))
}
