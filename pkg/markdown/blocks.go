package markdown

import "strings"

// Block is a fenced code block of a source, as its language shows it.
type Block struct {
	// Lang is the first word of the block's info string, in lower case.
	Lang string
	// Full is the markdown of the block shown in full.
	Full string
	// Collapsed is a line of markdown that shows in place of the block
	// until the reader opens it, or "" when the block always shows in
	// full.
	Collapsed string
	// URL is a page that shows the block, for the open action, or "".
	URL string
	// fence is the block's opening fence without its info string, and
	// code what the block holds, for the render to highlight.
	fence, code string
}

// fenced returns how the fenced code block src, from its opening fence to
// its closing one, whose info string starts with lang, shows. It is where
// a language gets a way of its own to show, such as a diagram; for now
// every block, mermaid too, shows in full as code. Code longer than
// maxHighlightedLines shows as blocks of that many lines, since glamour's
// time grows with the square of a block's length.
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

// showBlock is fenced, and a test's own way to show blocks while it runs.
var showBlock = fenced

// Blocks returns the fenced code blocks of src in order. The index of a
// collapsible one is what [Renderer.Render] takes to show it in full.
func Blocks(src string) []Block {
	var out []Block
	scan(src, "", func(string) {}, func(_ int, b Block) { out = append(out, b) })
	return out
}
