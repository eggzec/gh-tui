package owner

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// styles are the page's own, built once per theme.
type styles struct {
	edge, focusEdge   ownerui.Paint
	title, focusTitle ownerui.Paint
	warning           ownerui.Paint
	// langs color the glyphs of languages, by name and color, as they are
	// met.
	langs map[string]ownerui.Paint
	// shared are the paints the page shares with the profile, the pinned
	// cards and the repositories.
	shared ownerui.Styles
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	return styles{
		langs:      map[string]ownerui.Paint{},
		edge:       ownerui.NewPaint(border),
		focusEdge:  ownerui.NewPaint(t.Accent),
		title:      ownerui.NewPaint(t.Muted),
		focusTitle: ownerui.NewPaint(t.Accent.Bold(true)),
		warning:    ownerui.NewPaint(t.Warning),
		shared: ownerui.Styles{
			Name:     ownerui.NewPaint(t.Title),
			Login:    ownerui.NewPaint(t.Muted),
			Text:     ownerui.NewPaint(t.Text),
			Muted:    ownerui.NewPaint(t.Muted),
			Subtle:   ownerui.NewPaint(t.Subtle),
			Accent:   ownerui.NewPaint(t.Accent),
			Selected: ownerui.NewPaint(t.Title),
			Cursor:   t.Accent.Render(ic.Cursor) + " ",
			Blurred:  t.Subtle.Render(ic.Cursor) + " ",
		},
	}
}

// langPaint returns the paint of the language glyph of the repository
// with language and color, built once per language and theme.
func (s *Section) langPaint(language, color string) ownerui.Paint {
	k := language + "\x00" + color
	if p, ok := s.st.langs[k]; ok {
		return p
	}
	p := ownerui.NewPaint(s.theme.Language(language, color))
	s.st.langs[k] = p
	return p
}
