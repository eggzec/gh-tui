package markdown

import (
	"regexp"
	"strings"

	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// headStyles are the styles of the head of a collapsible block, made from
// those of the markdown around it: its name as a link's text, its size as
// code, and its link as a link.
type headStyles struct {
	name, size, link, note, text lipgloss.Style
	// imageText and imageURL style an image shown as its text, as
	// glamour shows it.
	imageText, imageURL lipgloss.Style
}

func newHeadStyles(s ansi.StyleConfig) headStyles {
	code := primitive(s.CodeBlock.StylePrimitive)
	return headStyles{
		name: primitive(s.LinkText),
		size: code,
		link: primitive(s.Link),
		note: code.Italic(true),
		text: code,

		imageText: primitive(s.ImageText),
		imageURL:  primitive(s.Image),
	}
}

// image returns an image as its text, as the default style shows one:
// its alt text and, in brackets, its address.
func (h headStyles) image(alt, url string) string {
	return h.imageText.Render("🖼 "+termtext.OneLine(alt)) + " " + h.imageURL.Render("("+termtext.OneLine(url)+")")
}

// line returns the head of b, which is collapsible, as one styled line.
func (h headStyles) line(b Block) string {
	offer := h.link.Render(b.offer())
	if b.URL == "" {
		offer = h.note.Render(b.offer())
	}
	return h.name.Render("◆ "+b.kind) + h.size.Render(" · "+b.size()+" · ") + offer
}

// body returns the lines of code, plain, in the style of code blocks and
// indented under the head.
func (h headStyles) body(code string) []string {
	lines := strings.Split(code, "\n")
	for i, l := range lines {
		lines[i] = "  " + h.text.Render(l)
	}
	return lines
}

// primitive returns p as a lipgloss style, with its colors and
// attributes.
func primitive(p ansi.StylePrimitive) lipgloss.Style {
	s := lipgloss.NewStyle()
	if p.Color != nil {
		s = s.Foreground(lipgloss.Color(*p.Color))
	}
	if p.BackgroundColor != nil {
		s = s.Background(lipgloss.Color(*p.BackgroundColor))
	}
	if p.Bold != nil {
		s = s.Bold(*p.Bold)
	}
	if p.Italic != nil {
		s = s.Italic(*p.Italic)
	}
	if p.Underline != nil {
		s = s.Underline(*p.Underline)
	}
	return s
}

// viewURL matches the links that package mermaid makes, the only ones a
// head links to.
var viewURL = regexp.MustCompile(`^https://mermaid\.live/view#pako:[A-Za-z0-9_-]+$`)

// linked returns line, the head of a diagram made safe, with what it
// offers a hyperlink to url, which a terminal opens on a click. Lipgloss's
// own hyperlinks would be dropped with those of the source, so the link
// goes in after. Without the whole offer, as when the head was cut, or
// with a url of another kind, the line has no link.
func linked(line, url string) string {
	const first, last = "View", "↗"
	i, j := strings.Index(line, first), strings.LastIndex(line, last)
	if !viewURL.MatchString(url) || i < 0 || j < i {
		return line
	}
	j += len(last)
	if xansi.Strip(line[i:j]) != viewText {
		return line
	}
	return line[:i] + termtext.Link(url, line[i:j]) + line[j:]
}
