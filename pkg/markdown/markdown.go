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
	style ansi.StyleConfig
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
	// they are open.
	heads headStyles
}

// rendered is a render and the heads of its collapsible blocks.
type rendered struct {
	text  string
	heads []Head
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
	r := &Renderer{nonce: strconv.FormatUint(rand.Uint64(), 36)}
	r.SetStyle(style)
	return r
}

// SetStyle sets the style and forgets what was rendered in the old one.
func (r *Renderer) SetStyle(style ansi.StyleConfig) {
	var zero uint
	style.Document.Margin = &zero
	r.code = codeStyle(style.CodeBlock)
	r.heads = newHeadStyles(style)
	// Glamour would run chroma on any code, in any container, with any
	// lexer, or one it guesses, and chroma can't be stopped; the renderer
	// highlights instead, within its limits.
	style.CodeBlock.Chroma, style.CodeBlock.Theme = nil, ""
	r.style = style
	r.terms = nil
	r.recent, r.older = nil, nil
	r.highlights = nil
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
	if out, ok := r.recent[k]; ok {
		return out
	}
	out, ok := r.older[k]
	kept := true
	if !ok {
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

// Renders returns how many times the renderer ran glamour, so tests and
// benchmarks can tell a render from a cache hit.
func (r *Renderer) Renders() int { return r.renders }

// render renders src, and reports whether the render is worth keeping:
// it isn't if it left code plain that may highlight in another.
func (r *Renderer) render(src string, width int, open []int) (rendered, bool) {
	r.renders++
	b := newBudget()
	var parts []part
	text := prepare(src, open, r.hint, func(i int, blk Block, shown bool) string {
		// The marks are indented as the fence is, so they stay in the
		// block's list item.
		indent := blk.indent()
		if blk.Collapsed != "" {
			if len(indent) > 3 {
				// As deep as indented code, it may not be a block of its
				// own; glamour shows it.
				return plain(i, blk, shown)
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
	})
	lines, err := r.lines(text, width)
	var heads []Head
	if err == nil && len(parts) > 0 {
		var ok bool
		if lines, heads, ok = r.splice(lines, parts, width); !ok {
			lines, err = r.lines(prepare(src, open, r.hint, plain), width)
		}
	}
	if err != nil {
		// Showing the source beats showing nothing.
		text = prepare(src, open, r.hint, plain)
		lines = strings.Split(xansi.Wrap(text, width, ""), "\n")
		heads = nil
	}
	lines, front := trimBlank(lines)
	for i, l := range lines {
		lines[i] = safe(tidy(l))
	}
	for i := range heads {
		heads[i].Line -= front
		heads[i].End -= front
		// Only now, once the lines are safe, which drops every link.
		lines[heads[i].Line] = linked(lines[heads[i].Line], heads[i].URL)
	}
	return rendered{text: strings.Join(lines, "\n"), heads: heads}, !b.busy
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
		t, err = glamour.NewTermRenderer(
			glamour.WithStyles(r.style),
			glamour.WithWordWrap(width),
			// GitHub breaks the line where a comment or an issue does,
			// which templates rely on.
			glamour.WithPreservedNewLines(),
			glamour.WithEmoji(),
		)
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
