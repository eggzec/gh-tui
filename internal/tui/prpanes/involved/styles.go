package involved

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Styles are what the rows are drawn with. Build them once, with [NewStyles],
// and again when the theme or the icons change.
type Styles struct {
	glyphs ui.GlyphStyles
	dim    lipgloss.Style
	bold   lipgloss.Style
	// sep joins the roles of a person, and ellipsis ends a line that was cut.
	sep, ellipsis string
	// icons draw the rules.
	icons ui.Icons
}

// NewStyles returns the styles for theme t and the icons ic.
func NewStyles(t ui.Theme, ic ui.Icons) Styles {
	return Styles{
		glyphs: ui.NewGlyphStyles(t, ic),
		dim:    t.Subtle,
		bold:   lipgloss.NewStyle().Bold(true),
		sep:    ic.Separator, ellipsis: ic.Ellipsis,
		icons:  ic,
	}
}

// glyphOf returns the glyph of a reviewer in state s.
func glyphOf(s State) ui.Glyph {
	switch s {
	case None, Commented:
		return ui.GlyphCommented
	case Requested:
		return ui.GlyphRequested
	case Rerequested:
		return ui.GlyphRerequested
	case Approved:
		return ui.GlyphApproved
	case ChangesRequested:
		return ui.GlyphChanges
	case Dismissed:
		return ui.GlyphDismissed
	}
	return ui.GlyphCommented
}

// reviewer returns the line of a reviewer: the glyph of their state, their
// name, and the state in words.
func (s *Styles) reviewer(p Person) string {
	line := s.glyphs.Render(glyphOf(p.State)) + " " + p.Name
	if p.Team != "" {
		line += s.dim.Render(" team")
	}
	return line + "  " + s.dim.Render(p.State.String())
}
