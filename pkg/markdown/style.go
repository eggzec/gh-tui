package markdown

import (
	"charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
)

// DefaultStyle returns the style of markdown on a dark or light terminal:
// glamour's own, with text in the terminal's own color, which suits its
// background best, headings that read as headings rather than as the
// hashes that mark them in the source, and images shown as a line with
// their text and address.
func DefaultStyle(isDark bool) ansi.StyleConfig {
	s := glamourstyles.LightStyleConfig
	if isDark {
		s = glamourstyles.DarkStyleConfig
	}
	s.Document.Color = nil
	for _, h := range []*ansi.StyleBlock{&s.H2, &s.H3, &s.H4, &s.H5, &s.H6} {
		h.Prefix = ""
	}
	s.ImageText.Format = "🖼 {{.text}}"
	s.Image.Format = "({{.text}})"
	return s
}
