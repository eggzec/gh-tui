// Package markdown renders GitHub-flavoured markdown, such as the body of an
// issue or a comment, for the terminal with glamour. It makes the source
// safe to draw first, since it is untrusted, and shows what GitHub shows of
// the HTML that bodies often hold, such as the comments of a template or a
// collapsed section. It keeps what it rendered, so drawing the same text at
// the same width again costs a map lookup.
package markdown

import (
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
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
	recent, older map[key]string
	renders       int
	// hint is what the note that ends a cut source offers.
	hint string
}

type key struct {
	src   string
	width int
	open  string
}

// New returns a renderer of markdown in style. Its document margin is
// always zero: the caller indents the lines as its layout needs.
func New(style ansi.StyleConfig) *Renderer {
	r := &Renderer{}
	r.SetStyle(style)
	return r
}

// SetStyle sets the style and forgets what was rendered in the old one.
func (r *Renderer) SetStyle(style ansi.StyleConfig) {
	var zero uint
	style.Document.Margin = &zero
	r.style = style
	r.terms = nil
	r.recent, r.older = nil, nil
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
	if width <= 0 || strings.TrimSpace(src) == "" {
		return ""
	}
	k := key{src: src, width: width}
	if len(open) > 0 {
		k.open = fmt.Sprint(open)
	}
	if out, ok := r.recent[k]; ok {
		return out
	}
	out, ok := r.older[k]
	if !ok {
		out = r.render(src, width, open)
	}
	if len(src) > maxKept {
		return out
	}
	if len(r.recent) >= keep {
		r.older, r.recent = r.recent, nil
	}
	if r.recent == nil {
		r.recent = make(map[key]string)
	}
	r.recent[k] = out
	return out
}

// Renders returns how many times the renderer ran glamour, so tests and
// benchmarks can tell a render from a cache hit.
func (r *Renderer) Renders() int { return r.renders }

func (r *Renderer) render(src string, width int, open []int) string {
	r.renders++
	text := prepare(src, open, r.hint)
	out, err := r.glamour(text, width)
	if err != nil {
		// Showing the source beats showing nothing.
		out = xansi.Wrap(text, width, "")
	}
	lines := trimBlank(strings.Split(out, "\n"))
	for i, l := range lines {
		lines[i] = safe(tidy(l))
	}
	return strings.Join(lines, "\n")
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
			glamour.WithChromaFormatter("terminal16m"),
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

// trimBlank drops the blank lines around lines.
func trimBlank(lines []string) []string {
	blank := func(l string) bool { return strings.TrimSpace(xansi.Strip(l)) == "" }
	for len(lines) > 0 && blank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return lines
}
