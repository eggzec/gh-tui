package markdown

import (
	"hash/maphash"
	"regexp"
	"strconv"
	"strings"

	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	xansi "github.com/charmbracelet/x/ansi"
)

// maxHighlights is how many highlighted blocks a renderer keeps.
const maxHighlights = 256

// highlight returns the lines of the code of blk highlighted, if it is
// code the renderer highlights and b has room for it. Only a block whose
// fence is indented less than four spaces is, since only such a block is
// sure to be fenced code wherever it is; glamour shows the rest plain.
func (r *Renderer) highlight(blk Block, b *budget) ([]string, bool) {
	if r.code == nil || blk.fence == "" || len(blk.fence)-len(strings.TrimLeft(blk.fence, " ")) > 3 {
		return nil, false
	}
	l := lexer(blk.Lang, blk.code)
	if l == nil {
		return nil, false
	}
	k := verdictKey{lexer: l.Config().Name, code: maphash.String(verdictSeed, blk.code)}
	if lines, ok := r.highlights[k]; ok {
		return lines, true
	}
	toks, ok := tokens(l, blk.code, b)
	if !ok {
		return nil, false
	}
	var out strings.Builder
	if err := formatters.TTY16m.Format(&out, r.code, chroma.Literator(toks...)); err != nil {
		return nil, false
	}
	lines := strings.Split(out.String(), "\n")
	lines = lines[:min(len(lines), strings.Count(blk.code, "\n")+1)]
	if r.highlights == nil || len(r.highlights) >= maxHighlights {
		r.highlights = make(map[verdictKey][]string)
	}
	r.highlights[k] = lines
	return lines, true
}

// trailingStyle matches the style sequences that end a string.
var trailingStyle = regexp.MustCompile(`(?:\x1b\[[0-9;:]*m)+$`)

// mark returns the line that stands for part i in what glamour renders.
func (r *Renderer) mark(i int) string {
	return r.nonce + "." + strconv.Itoa(i) + "."
}

// part is what a mark stands for: the lines of highlighted code, the head
// of a collapsible block, the code of an open one, which follows its head,
// or an image that stands alone on its line.
type part struct {
	lines []string
	head  *Head
	body  bool
	pic   *picture
}

// picture is an image that stands alone on its line: its alt text and
// address.
type picture struct {
	alt, url string
}

// spliced is what splice made: the lines, the heads where they landed,
// the images that stand alone with what drew them, and the rows of the
// pictures drawn, which go in for their marks once the lines are safe.
type spliced struct {
	lines []string
	heads []Head
	slots []slot
	rows  []row
}

// row is a line of a picture: the line of the render it goes in, for the
// mark of n, and its text.
type row struct {
	line, n int
	text    string
}

// rowMark returns what stands for row n of the pictures until the lines
// are safe.
func (r *Renderer) rowMark(n int) string {
	return r.nonce + "!" + strconv.Itoa(n) + "!"
}

// splice returns lines with the line of each mark replaced by the lines
// of its part, each after what glamour put before the mark, such as the
// block's margin, and the heads, where they landed. Code is wrapped at
// width as glamour wraps code, and a head is cut to one line. An image
// takes the lines Pictures draws it in, in the room the margin leaves,
// or else its text, wrapped as code is. It reports false if glamour didn't render
// each mark as a line of its own, in order.
func (r *Renderer) splice(lines []string, parts []part, width int) (spliced, bool) {
	out := make([]string, 0, len(lines))
	var sp spliced
	next := 0
	for _, l := range lines {
		i := strings.Index(l, r.nonce+".")
		if i < 0 {
			out = append(out, l)
			continue
		}
		j := i + len(r.nonce) + 1
		k := j
		for k < len(l) && l[k] >= '0' && l[k] <= '9' {
			k++
		}
		if next >= len(parts) || k == j || k == len(l) || l[k] != '.' || l[j:k] != strconv.Itoa(next) {
			return spliced{}, false
		}
		if strings.ContainsAny(xansi.Strip(l[:i]), "`~") || strings.TrimSpace(xansi.Strip(l[k+1:])) != "" {
			return spliced{}, false
		}
		// The style glamour opened for the mark would only be overridden.
		prefix := trailingStyle.ReplaceAllString(l[:i], "")
		p := parts[next]
		next++
		if p.head != nil {
			h := *p.head
			h.Line = len(out)
			out = append(out, xansi.Truncate(prefix+p.lines[0], width, r.glyphs.Ellipsis)+"\x1b[0m")
			h.End = len(out)
			sp.heads = append(sp.heads, h)
			continue
		}
		room := max(width-xansi.StringWidth(prefix), 1)
		if p.pic != nil {
			pic := r.pictures(p.pic.url, room)
			sp.slots = append(sp.slots, slot{url: p.pic.url, width: room, lines: pic})
			if pic == nil {
				for w := range strings.SplitSeq(lipgloss.Wrap(r.heads.image(p.pic.alt, p.pic.url), room, ""), "\n") {
					out = append(out, xansi.Truncate(prefix+w, width, "")+"\x1b[0m")
				}
				continue
			}
			// The margin's style is closed before the picture, which must
			// keep its own colors.
			for _, text := range pic {
				sp.rows = append(sp.rows, row{line: len(out), n: len(sp.rows), text: text})
				out = append(out, prefix+"\x1b[0m"+r.rowMark(len(sp.rows)-1)+"\x1b[0m")
			}
			continue
		}
		for _, c := range p.lines {
			for w := range strings.SplitSeq(lipgloss.Wrap(c, room, ""), "\n") {
				out = append(out, xansi.Truncate(prefix+w, width, "")+"\x1b[0m")
			}
		}
		if p.body && len(sp.heads) > 0 {
			sp.heads[len(sp.heads)-1].End = len(out)
		}
	}
	sp.lines = out
	return sp, next == len(parts)
}

// codeStyle returns the chroma style of the code in s, as glamour would
// make it, or nil if s highlights nothing.
func codeStyle(s ansi.StyleCodeBlock) *chroma.Style {
	c := s.Chroma
	if c == nil {
		if s.Theme == "" {
			return nil
		}
		return chromastyles.Get(s.Theme)
	}
	style, err := chroma.NewStyle("gh-tui", chroma.StyleEntries{
		chroma.Text:                entry(c.Text),
		chroma.Error:               entry(c.Error),
		chroma.Comment:             entry(c.Comment),
		chroma.CommentPreproc:      entry(c.CommentPreproc),
		chroma.Keyword:             entry(c.Keyword),
		chroma.KeywordReserved:     entry(c.KeywordReserved),
		chroma.KeywordNamespace:    entry(c.KeywordNamespace),
		chroma.KeywordType:         entry(c.KeywordType),
		chroma.Operator:            entry(c.Operator),
		chroma.Punctuation:         entry(c.Punctuation),
		chroma.Name:                entry(c.Name),
		chroma.NameBuiltin:         entry(c.NameBuiltin),
		chroma.NameTag:             entry(c.NameTag),
		chroma.NameAttribute:       entry(c.NameAttribute),
		chroma.NameClass:           entry(c.NameClass),
		chroma.NameConstant:        entry(c.NameConstant),
		chroma.NameDecorator:       entry(c.NameDecorator),
		chroma.NameException:       entry(c.NameException),
		chroma.NameFunction:        entry(c.NameFunction),
		chroma.NameOther:           entry(c.NameOther),
		chroma.Literal:             entry(c.Literal),
		chroma.LiteralNumber:       entry(c.LiteralNumber),
		chroma.LiteralDate:         entry(c.LiteralDate),
		chroma.LiteralString:       entry(c.LiteralString),
		chroma.LiteralStringEscape: entry(c.LiteralStringEscape),
		chroma.GenericDeleted:      entry(c.GenericDeleted),
		chroma.GenericEmph:         entry(c.GenericEmph),
		chroma.GenericInserted:     entry(c.GenericInserted),
		chroma.GenericStrong:       entry(c.GenericStrong),
		chroma.GenericSubheading:   entry(c.GenericSubheading),
		chroma.Background:          entry(c.Background),
	})
	if err != nil {
		return nil
	}
	return style
}

// entry returns p as a chroma style entry.
func entry(p ansi.StylePrimitive) string {
	var parts []string
	if p.Color != nil {
		parts = append(parts, *p.Color)
	}
	if p.BackgroundColor != nil {
		parts = append(parts, "bg:"+*p.BackgroundColor)
	}
	for _, f := range []struct {
		on   *bool
		name string
	}{{p.Italic, "italic"}, {p.Bold, "bold"}, {p.Underline, "underline"}} {
		if f.on != nil && *f.on {
			parts = append(parts, f.name)
		}
	}
	return strings.Join(parts, " ")
}
