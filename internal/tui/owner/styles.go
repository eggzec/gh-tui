package owner

import (
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// styles are the page's own, built once per theme.
type styles struct {
	// frames draws the boxes and the strip of panes.
	frames  panes.Styles
	warning ownerui.Paint
	// tab marks the tab on view in the list's tab line.
	tab ownerui.Paint
	// langs color the glyphs of languages, by name and color, as they are
	// met.
	langs map[string]ownerui.Paint
	// shared are the paints the page shares with the profile, the pinned
	// cards and the repositories.
	shared ownerui.Styles
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	frames := panes.DefaultStyles(t.Dark)
	frames.Border = ic.Border
	frames.Ellipsis = ic.Ellipsis
	frames.Separator = ic.Separator
	return styles{
		langs:   map[string]ownerui.Paint{},
		frames:  frames,
		warning: ownerui.NewPaint(t.Warning),
		tab:     ownerui.NewPaint(t.Accent.Bold(true)),
		shared: ownerui.Styles{
			Name:    ownerui.NewPaint(t.Title),
			Login:   ownerui.NewPaint(t.Muted),
			Text:    ownerui.NewPaint(t.Text),
			Muted:   ownerui.NewPaint(t.Muted),
			Subtle:  ownerui.NewPaint(t.Subtle),
			Accent:  ownerui.NewPaint(t.Accent),
			Rows:    ownerui.NewRowPaints(t),
			Cursor:  t.Accent.Render(ic.Cursor) + " ",
			Blurred: t.Subtle.Render(ic.Cursor) + " ",
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
