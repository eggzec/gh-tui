package markdown

import (
	"charm.land/glamour/v2/ansi"
	glamourstyles "charm.land/glamour/v2/styles"
)

// DefaultStyle returns the style of markdown on a dark or light terminal:
// glamour's own, with text in the terminal's own color, which suits its
// background best, headings that read as headings rather than as the
// hashes that mark them in the source, and images shown as a line with
// their text and address. On a light terminal code takes GitHub's light
// colors, since glamour's are too pale to read on white.
func DefaultStyle(isDark bool) ansi.StyleConfig {
	s := glamourstyles.LightStyleConfig
	if isDark {
		s = glamourstyles.DarkStyleConfig
	} else {
		s.Code.Color = new("124")
		s.CodeBlock.Chroma = lightCode()
	}
	s.Document.Color = nil
	for _, h := range []*ansi.StyleBlock{&s.H2, &s.H3, &s.H4, &s.H5, &s.H6} {
		h.Prefix = ""
	}
	// The wrap glamour gives a title keeps the space before it on the line
	// of a word too long for one, so the line came out a cell wider than
	// the width, and in a quote the quote's own wrap then pushed what
	// didn't fit out from behind the bar. A space that doesn't break is
	// part of the word, and the wrap breaks it within the width.
	s.H1.Prefix = "\u00a0"
	s.ImageText.Format = unicodeImage + " {{.text}}"
	s.Image.Format = "({{.text}})"
	// Glamour counts a quote's indent as its Indent cells, however wide
	// the token it draws them with, so "│ " once made each quoted line a
	// cell wider than the width. The bar is drawn twice instead, which is
	// as wide as glamour counts, and quoteBars makes the second a space.
	s.BlockQuote.Indent = new(uint(2))
	s.BlockQuote.IndentToken = new(quoteToken)
	return s
}

// quoteToken is what glamour draws each cell of a quote's indent with in
// the default glyphs, which the glyphs of a renderer replace.
const quoteToken = unicodeQuote + "\u200b"

// lightCode returns the colors of highlighted code on a light terminal,
// GitHub's, each of which reads on white. It sets no background, so the
// code sits on the terminal's own.
func lightCode() *ansi.Chroma {
	c := func(color string) ansi.StylePrimitive { return ansi.StylePrimitive{Color: new(color)} }
	const (
		text     = "#1f2328"
		comment  = "#59636e"
		keyword  = "#cf222e"
		constant = "#0550ae"
		entity   = "#6639ba"
		str      = "#0a3069"
		variable = "#953800"
		tag      = "#116329"
		deleted  = "#82071e"
	)
	return &ansi.Chroma{
		Text:                c(text),
		Error:               ansi.StylePrimitive{Color: new(deleted), BackgroundColor: new("#ffebe9")},
		Comment:             c(comment),
		CommentPreproc:      c(keyword),
		Keyword:             c(keyword),
		KeywordReserved:     c(keyword),
		KeywordNamespace:    c(keyword),
		KeywordType:         c(constant),
		Operator:            c(keyword),
		Punctuation:         c(text),
		Name:                c(text),
		NameBuiltin:         c(constant),
		NameTag:             c(tag),
		NameAttribute:       c(constant),
		NameClass:           c(variable),
		NameConstant:        c(constant),
		NameDecorator:       c(entity),
		NameException:       c(variable),
		NameFunction:        c(entity),
		NameOther:           c(text),
		Literal:             c(constant),
		LiteralNumber:       c(constant),
		LiteralDate:         c(constant),
		LiteralString:       c(str),
		LiteralStringEscape: c(tag),
		GenericDeleted:      c(deleted),
		GenericEmph:         ansi.StylePrimitive{Italic: new(true)},
		GenericInserted:     c(tag),
		GenericStrong:       ansi.StylePrimitive{Bold: new(true)},
		GenericSubheading:   c(constant),
	}
}
