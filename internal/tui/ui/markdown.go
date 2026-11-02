package ui

import "github.com/eggzec/gh-tui/pkg/markdown"

// NewMarkdown returns a markdown renderer in the style of the theme, drawing
// what it adds of its own with the icons' glyphs and links in the theme's
// link color, so every screen renders markdown the same way.
func NewMarkdown(t Theme, ic Icons) *markdown.Renderer {
	md := markdown.New(t.Thread(ic).Markdown)
	md.SetGlyphs(ic.Markdown)
	md.SetLinkColor(t.LinkColor())
	return md
}
