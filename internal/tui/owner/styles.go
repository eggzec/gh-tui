package owner

import (
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// styles are the page's own, built once per theme.
type styles struct {
	// frames draws the boxes and the strip of panes.
	frames  panes.Styles
	warning ui.Paint
	// tab marks the tab on view in the list's tab line.
	tab ui.Paint
	// langs color the glyphs of languages, by name and color, as they are
	// met.
	langs map[string]ui.Paint
	// shared are the paints the page shares with the profile, the pinned
	// cards and the repositories.
	shared ui.ListStyles
}

func newStyles(t ui.Theme, ic ui.Icons) styles {
	frames := panes.DefaultStyles(t.Dark)
	frames.Border = ic.Border
	frames.Ellipsis = ic.Ellipsis
	frames.Separator = ic.Separator
	return styles{
		langs:   map[string]ui.Paint{},
		frames:  frames,
		warning: ui.NewPaint(t.Warning),
		tab:     ui.NewPaint(t.Accent.Bold(true)),
		shared: ui.ListStyles{
			Name:    ui.NewPaint(t.Title),
			Login:   ui.NewPaint(t.Muted),
			Text:    ui.NewPaint(t.Text),
			Muted:   ui.NewPaint(t.Muted),
			Subtle:  ui.NewPaint(t.Subtle),
			Accent:  ui.NewPaint(t.Accent),
			Rows:    ui.NewRowPaints(t),
			Cursor:  t.Accent.Render(ic.Cursor) + " ",
			Blurred: t.Subtle.Render(ic.Cursor) + " ",
		},
	}
}

// langPaint returns the paint of the language glyph of the repository
// with language and color, built once per language and theme.
func (s *Section) langPaint(language, color string) ui.Paint {
	k := language + "\x00" + color
	if p, ok := s.st.langs[k]; ok {
		return p
	}
	p := ui.NewPaint(s.theme.Language(language, color))
	s.st.langs[k] = p
	return p
}
