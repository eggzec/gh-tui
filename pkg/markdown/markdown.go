// Package markdown renders GitHub-flavoured markdown, such as the body of an
// issue or a comment, for the terminal with glamour. It makes the source
// safe to draw first, since it is untrusted, and shows what GitHub shows of
// the HTML that bodies often hold, such as the comments of a template or a
// collapsed section. It keeps what it rendered, so drawing the same text at
// the same width again costs a map lookup.
package markdown

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"github.com/alecthomas/chroma/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// keep is how many renders each generation of the cache holds. A render
// used in the last two generations stays, so a thread's loaded comments,
// which are a few chunks, stay rendered while it is open.
const keep = 256

// maxKept is the longest source the cache keeps a render of, so a few
// huge bodies can't fill memory. A longer one renders each time it is
// asked for, which is rare, since the thread keeps its lines.
const maxKept = 256 << 10

// Renderer renders markdown in a style at any width, and keeps what it
// rendered until the style changes. Create it with [New]. It is not safe
// for concurrent use; a bubble calls it from Update.
type Renderer struct {
	// base is the style as it was set, and style the one glamour renders
	// in: base with the renderer's changes and the glyphs that it draws.
	base, style ansi.StyleConfig
	// glyphs are what the render draws of its own, with the defaults
	// where they were left empty.
	glyphs Glyphs
	// terms holds a glamour renderer per width, since making one is costly
	// and a view may render at two widths, such as a body and its
	// comments.
	terms map[int]*glamour.TermRenderer
	// recent and older are the two generations of the cache. A hit in
	// older moves to recent; when recent is full it becomes older.
	recent, older map[key]rendered
	renders       int
	// hint is what the note that ends a cut source offers.
	hint string
	// code is the style of highlighted code, or nil for none, and
	// highlights keeps highlighted code, which is the same at any width.
	code       *chroma.Style
	highlights map[verdictKey][]string
	// nonce makes the marks that stand for highlighted code and the heads
	// of collapsible blocks in what glamour renders.
	nonce string
	// heads styles the heads of collapsible blocks, and their code while
	// they are open, and the images shown as their text.
	heads headStyles
	// pictures draws the images that stand alone on their lines, or is
	// nil when none are drawn, and relative passes it those by relative
	// addresses too.
	pictures Pictures
	relative bool
}

// Pictures returns the lines that draw the image at url in at most width
// cells, or nil while it can't, such as before the image arrived, when
// the image shows as it does without pictures: its alt text and address.
// The lines must each be as wide as the others, at most width cells, and
// are put in the render as they are, after the styles of the markdown
// around them were closed: they never go through a style or a wrap. The
// renderer asks again each time it would reuse a render with images, and
// renders again when the lines changed, so they must change only when
// what they draw does.
type Pictures func(url string, width int) []string

// rendered is a render, the heads of its collapsible blocks and the
// images that stand alone on their lines, with what drew them.
type rendered struct {
	text  string
	heads []Head
	slots []slot
}

// slot is an image that stands alone on its lines, the room it had, and
// the lines Pictures gave for it, nil when it showed as its text.
type slot struct {
	url   string
	width int
	lines []string
}

// Head is the line that stands for a collapsible block, such as a
// diagram, in a render: the block's kind, size and link, which the reader
// opens to see its code under it.
type Head struct {
	// Block is the block's index among the [Blocks] of the source, which
	// [Renderer.Render] takes to show it open.
	Block int
	// Line is the head's line in the render, and End the line after the
	// block's code while it is open, or after the head.
	Line, End int
	// URL is a page that shows the block, or "" if it has none.
	URL string
}

type key struct {
	src   string
	width int
	open  string
}

// New returns a renderer of markdown in style. Its document margin is
// always zero: the caller indents the lines as its layout needs.
func New(style ansi.StyleConfig) *Renderer {
	r := &Renderer{nonce: strconv.FormatUint(rand.Uint64(), 36), glyphs: Glyphs{}.orDefault()}
	r.SetStyle(style)
	return r
}

// SetStyle sets the style and forgets what was rendered in the old one.
func (r *Renderer) SetStyle(style ansi.StyleConfig) {
	r.base = style
	r.restyle()
}

// SetGlyphs sets what the render draws of its own, such as bullets and
// quote bars, and forgets what was rendered when they change. The zero
// Glyphs draws the defaults.
func (r *Renderer) SetGlyphs(g Glyphs) {
	if g = g.orDefault(); g == r.glyphs {
		return
	}
	r.glyphs = g
	r.restyle()
}

// restyle makes the style glamour renders in from the base style and the
// glyphs, and forgets what was rendered.
func (r *Renderer) restyle() {
	style := r.glyphs.style(r.base)
	var zero uint
	style.Document.Margin = &zero
	r.code = codeStyle(style.CodeBlock)
	r.heads = newHeadStyles(style, r.glyphs)
	// Glamour would run chroma on any code, in any container, with any
	// lexer, or one it guesses, and chroma can't be stopped; the renderer
	// highlights instead, within its limits.
	style.CodeBlock.Chroma, style.CodeBlock.Theme = nil, ""
	r.style = style
	r.terms = nil
	r.recent, r.older = nil, nil
	r.highlights = nil
}

// SetPictures draws, with p, the images that stand alone on their lines,
// as text alone in a paragraph or an img tag on a line of its own,
// instead of their alt text and address. It forgets what was rendered
// when pictures begin or stop being drawn; a kept render whose pictures p
// draws otherwise than the one before renders again when next asked for.
// An image in running text, or in a link, such as a badge, stays text. A
// nil p draws none, and the renders are what they were without.
func (r *Renderer) SetPictures(p Pictures) {
	if (p == nil) != (r.pictures == nil) {
		r.recent, r.older = nil, nil
	}
	r.pictures = p
}

// SetRelativePictures passes Pictures the images by relative addresses
// that stand alone on their lines too, such as img/logo.png, for a caller
// that knows what they are relative to, such as a file of a repository.
// Without it, only images on the web are drawn, and the rest stay text.
// It forgets what was rendered when it changes.
func (r *Renderer) SetRelativePictures(on bool) {
	if on != r.relative {
		r.relative = on
		r.recent, r.older = nil, nil
	}
}

// SetHint sets what the note that ends a source too long to show in full
// offers, such as "o to open on GitHub", and forgets what was rendered.
// The default offers nothing.
func (r *Renderer) SetHint(hint string) {
	if hint == r.hint {
		return
	}
	r.hint = hint
	r.recent, r.older = nil, nil
}

// Render returns src rendered as lines at most width cells wide, without
// the blank lines around them, or "" when there is nothing to show. The
// lines are not padded to the width. Code wider than width stays as wide
// as it is, for the caller to cut. The collapsible blocks of src show
// collapsed, except those whose index among its [Blocks] is in open.
func (r *Renderer) Render(src string, width int, open ...int) string {
	return r.get(src, width, open).text
}

// RenderHeads is [Renderer.Render], and returns the head of each
// collapsible block the render shows too, in order, for the caller to
// open or follow. A block glamour renders on its own, such as one in a
// quote, shows as code or as its collapsed line, without a head. The heads
// are shared: don't change them.
func (r *Renderer) RenderHeads(src string, width int, open ...int) (string, []Head) {
	out := r.get(src, width, open)
	return out.text, out.heads
}

func (r *Renderer) get(src string, width int, open []int) rendered {
	if width <= 0 || strings.TrimSpace(src) == "" {
		return rendered{}
	}
	k := key{src: src, width: width}
	if len(open) > 0 {
		k.open = fmt.Sprint(open)
	}
	if out, ok := r.recent[k]; ok && r.drawn(out) {
		return out
	}
	out, ok := r.older[k]
	kept := true
	if !ok || !r.drawn(out) {
		out, kept = r.render(src, width, open)
	}
	if !kept || len(src) > maxKept {
		return out
	}
	if len(r.recent) >= keep {
		r.older, r.recent = r.recent, nil
	}
	if r.recent == nil {
		r.recent = make(map[key]rendered)
	}
	r.recent[k] = out
	return out
}

// drawn reports whether out shows its images as Pictures would draw them
// now, so a render whose image arrived since, or changed, is made again:
// the lines of each image are part of what the render is kept under.
func (r *Renderer) drawn(out rendered) bool {
	for _, s := range out.slots {
		if r.pictures == nil || !slices.Equal(r.pictures(s.url, s.width), s.lines) {
			return false
		}
	}
	return true
}

// Renders returns how many times the renderer ran glamour, so tests and
// benchmarks can tell a render from a cache hit.
func (r *Renderer) Renders() int { return r.renders }

// render renders src, and reports whether the render is worth keeping:
// it isn't if it left code plain that may highlight in another. Where
// Pictures draws any of the images that stand alone on their lines, they
// show in lines of their own; while it draws none of them, the render is
// the one without pictures, so an image shows as its text, as it does
// without them, until it can be drawn.
func (r *Renderer) render(src string, width int, open []int) (rendered, bool) {
	if r.pictures == nil || !mayHaveImages(src) {
		return r.renderWith(src, width, open, false)
	}
	pics, kept := r.renderWith(src, width, open, true)
	if len(pics.slots) == 0 || slices.ContainsFunc(pics.slots, func(s slot) bool { return s.lines != nil }) {
		// Without slots the source rendered as it does without pictures.
		return pics, kept
	}
	out, plainKept := r.renderWith(src, width, open, false)
	out.slots = pics.slots
	return out, kept && plainKept
}

// mayHaveImages reports whether src may hold an image that stands alone
// on its line.
func mayHaveImages(src string) bool {
	return strings.Contains(src, "![") || strings.Contains(strings.ToLower(src), "<img")
}

// renderWith renders src, with the images that stand alone on their lines
// in lines of their own, as Pictures draws them, if pics is set.
func (r *Renderer) renderWith(src string, width int, open []int, pics bool) (rendered, bool) {
	r.renders++
	// Only glamour may draw the token that marks a quote's indent, or the
	// text could pass for a quote.
	g := r.glyphs
	src = strings.ReplaceAll(src, g.quoteToken(), g.Quote)
	b := newBudget()
	var parts []part
	text := prepare(src, open, r.hint, g, func(i int, blk Block, shown bool) string {
		// The marks are indented as the fence is, so they stay in the
		// block's list item.
		indent := blk.indent()
		if blk.Collapsed != "" {
			if len(indent) > 3 {
				// As deep as indented code, it may not be a block of its
				// own; glamour shows it.
				return r.plain(i, blk, shown)
			}
			// The head is a code block, which glamour puts on lines of its
			// own even in a list item, where it runs paragraphs together.
			parts = append(parts, part{lines: []string{r.heads.line(blk)}, head: &Head{Block: i, URL: blk.URL}})
			s := blk.fence + "\n" + indent + r.mark(len(parts)-1)
			if shown {
				parts = append(parts, part{lines: r.heads.body(blk.code), body: true})
				s += "\n" + indent + r.mark(len(parts)-1)
			}
			return s + "\n" + blk.fence
		}
		lines, ok := r.highlight(blk, b)
		if !ok {
			return blk.Full
		}
		parts = append(parts, part{lines: lines})
		return blk.fence + "\n" + indent + r.mark(len(parts)-1) + "\n" + blk.fence
	}, r.picture(pics, &parts), r.relative)
	lines, err := r.lines(text, width)
	var sp spliced
	if err == nil && len(parts) > 0 {
		var ok bool
		if sp, ok = r.splice(lines, parts, width); ok {
			lines = sp.lines
		} else {
			sp = spliced{}
			lines, err = r.lines(prepare(src, open, r.hint, g, r.plain, nil, false), width)
		}
	}
	if err != nil {
		// Showing the source beats showing nothing.
		text = prepare(src, open, r.hint, g, r.plain, nil, false)
		lines = strings.Split(xansi.Wrap(text, width, ""), "\n")
		sp = spliced{}
	}
	lines, front := trimBlank(lines)
	for i, l := range lines {
		lines[i] = safe(tidy(quoteBars(l, g)))
	}
	if g.ASCII {
		asciiLines(lines, g, func(i int) bool { return sp.code[i+front] })
	}
	heads := sp.heads
	for i := range heads {
		heads[i].Line -= front
		heads[i].End -= front
		// Only now, once the lines are safe, which drops every link.
		lines[heads[i].Line] = linked(lines[heads[i].Line], heads[i].URL, g)
	}
	// The lines of pictures go in only now too, since what makes a line
	// safe would take the characters that draw them for the text's own.
	for _, row := range sp.rows {
		if i := row.line - front; i >= 0 && i < len(lines) {
			lines[i] = strings.Replace(lines[i], r.rowMark(row.n), row.text, 1)
		}
	}
	return rendered{text: strings.Join(lines, "\n"), heads: heads, slots: sp.slots}, !b.busy
}

// picture returns what prepare shows an image alone on its line as: a
// block of its own whose mark splice turns into the image's lines, and
// adds its part to parts, if pics is set, or nil, which leaves the image
// as markdown.
func (r *Renderer) picture(pics bool, parts *[]part) func(alt, url string) string {
	if !pics {
		return nil
	}
	return func(alt, url string) string {
		*parts = append(*parts, part{pic: &picture{alt: alt, url: url}})
		// A code block, which glamour puts on lines of its own even
		// within a paragraph.
		return "```\n" + r.mark(len(*parts)-1) + "\n```"
	}
}

// lines returns text rendered by glamour at width, a line at a time.
func (r *Renderer) lines(text string, width int) ([]string, error) {
	out, err := r.glamour(text, width)
	if err != nil {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

func (r *Renderer) glamour(text string, width int) (string, error) {
	t, ok := r.terms[width]
	if !ok {
		var err error
		opts := []glamour.TermRendererOption{
			glamour.WithStyles(r.style),
			glamour.WithWordWrap(width),
			// GitHub breaks the line where a comment or an issue does,
			// which templates rely on.
			glamour.WithPreservedNewLines(),
		}
		if r.glyphs.ASCII {
			// The list of a table's links under it cuts a long address
			// with "…"; in its cell it wraps.
			opts = append(opts, glamour.WithInlineTableLinks(true))
		} else {
			opts = append(opts, glamour.WithEmoji())
		}
		t, err = glamour.NewTermRenderer(opts...)
		if err != nil {
			return "", err
		}
		if len(r.terms) >= 4 {
			r.terms = nil
		}
		if r.terms == nil {
			r.terms = make(map[int]*glamour.TermRenderer)
		}
		r.terms[width] = t
	}
	return t.Render(text)
}

// Room returns the width left for markdown in width after margin cells,
// at least one cell when width is positive, so a narrow view still shows
// the text, and 0 when there is no room at all.
func Room(width, margin int) int {
	if width <= 0 {
		return 0
	}
	return max(width-margin, 1)
}

// Indent returns the lines of rendered, each after prefix.
func Indent(rendered, prefix string) string {
	if rendered == "" {
		return ""
	}
	return prefix + strings.ReplaceAll(rendered, "\n", "\n"+prefix)
}

// trimBlank drops the blank lines around lines, and returns how many it
// dropped before them.
func trimBlank(lines []string) (trimmed []string, front int) {
	blank := func(l string) bool { return strings.TrimSpace(xansi.Strip(l)) == "" }
	for len(lines) > 0 && blank(lines[0]) {
		lines = lines[1:]
		front++
	}
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return lines, front
}
