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
	// Glamour counts a quote's indent as its Indent cells, however wide
	// the token it draws them with, so "│ " once made each quoted line a
	// cell wider than the width. The bar is drawn twice instead, which is
	// as wide as glamour counts, and quoteBars makes the second a space.
	s.BlockQuote.Indent = new(uint(2))
	s.BlockQuote.IndentToken = new(quoteToken)
	return s
}

// quoteToken is what glamour draws each cell of a quote's indent with: a
// bar, marked by a zero-width space as glamour's rather than the text's,
// so quoteBars changes only the indent.
const quoteToken = quoteBar + "\u200b"

// quoteBar is the bar that shows a quote.
const quoteBar = "│"
