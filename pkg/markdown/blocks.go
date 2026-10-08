package markdown

import (
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/pkg/mermaid"
)

// Block is a fenced code block of a source, as its language shows it.
type Block struct {
	// Lang is the first word of the block's info string, in lower case.
	Lang string
	// Full is the markdown of the block shown in full.
	Full string
	// Collapsed is a line of markdown that shows in place of the block
	// until the reader opens it, drawn in the default glyphs, or "" when
	// the block always shows in full. A renderer draws the line in its
	// own glyphs.
	Collapsed string
	// URL is a page that shows the block, for the open action, or "".
	URL string
	// fence is the block's opening fence without its info string, and
	// code what the block holds, for the render to highlight.
	fence, code string
	// kind names the diagram a collapsible block draws, and lines counts
	// its lines, for the render to draw its head.
	kind  string
	lines int
}

// indent returns the spaces that indent the block's fence.
func (b Block) indent() string {
	return b.fence[:len(b.fence)-len(strings.TrimLeft(b.fence, " "))]
}

// fenced returns how the fenced code block src, from its opening fence to
// its closing one, whose info string starts with lang, shows. It is where
// a language gets a way of its own to show: a mermaid diagram collapses to
// a line that names its kind and links to it on mermaid.live, and every
// other block shows in full as code. Code longer than maxHighlightedLines
// shows as blocks of that many lines, since glamour's time grows with the
// square of a block's length.
func fenced(lang, src string) Block {
	lines := strings.Split(src, "\n")
	f, ok := openFence(lines[0])
	if !ok {
		return Block{Lang: lang, Full: src}
	}
	fence := lines[0][:len(lines[0])-len(strings.TrimLeft(lines[0], " "))] + strings.Repeat(string(f.char), f.n)
	body := lines[1:]
	if n := len(body); n > 0 && f.closedBy(body[n-1]) {
		body = body[:n-1]
	}
	b := Block{Lang: lang, fence: fence, code: strings.Join(body, "\n")}
	if lang == "mermaid" {
		// The code of a block in a list item is indented as its fence is.
		b.code = dedent(body, len(b.indent()))
		b.kind, b.lines, b.URL = mermaid.Kind(b.code), len(body), mermaid.ViewURL(b.code)
		b.Collapsed = Glyphs{}.orDefault().collapsed(b)
	}
	var out strings.Builder
	for out.Len() == 0 || len(body) > 0 {
		n := min(len(body), maxHighlightedLines)
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(fence + lang + "\n")
		for _, l := range body[:n] {
			out.WriteString(l + "\n")
		}
		out.WriteString(fence)
		body = body[n:]
	}
	b.Full = out.String()
	return b
}

// dedent returns lines without up to n spaces that start each, joined.
func dedent(lines []string, n int) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		k := 0
		for k < n && k < len(l) && l[k] == ' ' {
			k++
		}
		out[i] = l[k:]
	}
	return strings.Join(out, "\n")
}

// size returns how many lines a diagram has, in words.
func (b Block) size() string {
	if b.lines == 1 {
		return "1 line"
	}
	return strconv.Itoa(b.lines) + " lines"
}

// offer returns what the head of a diagram offers in the glyphs g: its
// link, or why it has none.
func (b Block) offer(g Glyphs) string {
	if b.URL == "" {
		return "too large to view"
	}
	return g.viewText()
}

// showBlock is fenced, and a test's own way to show blocks while it runs.
var showBlock = fenced

// Blocks returns the fenced code blocks of src in order. The index of a
// collapsible one is what [Renderer.Render] takes to show it in full.
func Blocks(src string) []Block {
	var out []Block
	scan(src, "", Glyphs{}.orDefault(), 0, func(string, bool) {}, func(_ int, b Block) { out = append(out, b) })
	return out
}
