// Package desc turns the body of a pull request into the rows of its
// description pane: rendered markdown, one row per line, with the collapsible
// blocks of the body as folds. It is pure: text in, [foldtree.Row]s out.
// Loading, fold state and the view belong to the host.
package desc

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

const (
	// maxDepth is the deepest a details block nests. A block inside it opens
	// no fold of its own, so a hostile body cannot push its text out of the
	// width with guides.
	maxDepth = 16
	// maxSummary is how far into the text after a details tag its summary may
	// end, which keeps the search for it short on a body of tags.
	maxSummary = 1024
)

// Node is a part of a body: a run of markdown text, or a details block.
type Node struct {
	// Text is the markdown of a run of text, when Block is nil.
	Text string
	// Block is a details block.
	Block *Block
}

// Block is a <details> block.
type Block struct {
	// ID is the path of the block in the body: "d0", "d1" at the top, "d0.1"
	// for the second block inside the first.
	ID string
	// Summary is the text of its <summary>, with the tags dropped and the
	// spaces trimmed, or "" when it has none.
	Summary string
	// Nodes are what the block holds.
	Nodes []Node
}

var (
	tagRe     = regexp.MustCompile(`(?i)<(/?)details(?:\s[^>]*)?>`)
	summaryRe = regexp.MustCompile(`(?is)^\s*<summary(?:\s[^>]*)?>(.*?)</summary\s*>`)
	anyTagRe  = regexp.MustCompile(`<[^>]*>`)
)

// Parse splits body into text and details blocks. A tag inside a code fence is
// text, a block that is never closed runs to the end of the body, and a
// closing tag with nothing to close is dropped. Runs of text that are only
// space are dropped too.
func Parse(body string) []Node {
	p := parser{src: body, tags: tags(body)}
	return p.nodes("", 0)
}

// tags returns the indexes of the details tags in src that are not in a code
// fence, as the submatch indexes of [tagRe].
func tags(src string) [][]int {
	all := tagRe.FindAllStringSubmatchIndex(src, -1)
	if len(all) == 0 {
		return nil
	}
	fences := fenced(src)
	if len(fences) == 0 {
		return all
	}
	out := all[:0]
	f := 0
	for _, t := range all {
		for f < len(fences) && fences[f][1] <= t[0] {
			f++
		}
		if f < len(fences) && fences[f][0] <= t[0] {
			continue
		}
		out = append(out, t)
	}
	return out
}

// fenced returns the byte ranges of src that are code fences, the fence lines
// included, in order. A fence that is never closed runs to the end.
func fenced(src string) [][2]int {
	var (
		out   [][2]int
		open  byte
		width int
		start int
	)
	for off := 0; off < len(src); {
		end := len(src)
		if i := strings.IndexByte(src[off:], '\n'); i >= 0 {
			end = off + i
		}
		line := src[off:end]
		trimmed := strings.TrimLeft(line, " ")
		c, n := run(trimmed)
		ok := len(line)-len(trimmed) <= 3 && n >= 3
		switch {
		case open == 0 && ok && (c != '`' || !strings.Contains(trimmed[n:], "`")):
			open, width, start = c, n, off
		case open != 0 && ok && c == open && n >= width && strings.TrimSpace(trimmed[n:]) == "":
			out = append(out, [2]int{start, end})
			open = 0
		}
		off = end + 1
	}
	if open != 0 {
		out = append(out, [2]int{start, len(src)})
	}
	return out
}

// run returns the fence character that s starts with and how many of it
// there are, or 0 and 0.
func run(s string) (char byte, n int) {
	if s == "" || (s[0] != '`' && s[0] != '~') {
		return 0, 0
	}
	n = 1
	for n < len(s) && s[n] == s[0] {
		n++
	}
	return s[0], n
}

type parser struct {
	src  string
	tags [][]int
	pos  int // the start of the text not yet taken
	next int // the first tag not yet read
}

// nodes reads the nodes of the block id, at depth, up to its closing tag.
func (p *parser) nodes(id string, depth int) []Node {
	var (
		out    []Node
		blocks int
		// skipped counts the blocks that open no fold, being too deep.
		skipped int
	)
	text := func(to int) {
		if s := p.src[p.pos:to]; strings.TrimSpace(s) != "" {
			out = append(out, Node{Text: s})
		}
	}
	for p.next < len(p.tags) {
		t := p.tags[p.next]
		p.next++
		closing := t[3] > t[2]
		if closing && skipped > 0 {
			skipped--
			text(t[0])
			p.pos = t[1]
			continue
		}
		if closing {
			text(t[0])
			p.pos = t[1]
			if depth > 0 {
				return out
			}
			continue
		}
		if depth >= maxDepth {
			// Its summary stays, as a line of text.
			skipped++
			text(t[0])
			p.pos = t[1]
			if s := p.summary(); s != "" {
				out = append(out, Node{Text: s})
			}
			continue
		}
		text(t[0])
		p.pos = t[1]
		b := &Block{ID: child(id, blocks)}
		blocks++
		b.Summary = p.summary()
		b.Nodes = p.nodes(b.ID, depth+1)
		out = append(out, Node{Block: b})
	}
	text(len(p.src))
	p.pos = len(p.src)
	return out
}

// summary reads the <summary> that follows a details tag, if there is one.
func (p *parser) summary() string {
	rest := p.src[p.pos:]
	rest = rest[:min(len(rest), maxSummary)]
	m := summaryRe.FindStringSubmatchIndex(rest)
	if m == nil {
		return ""
	}
	p.pos += m[1]
	for p.next < len(p.tags) && p.tags[p.next][0] < p.pos {
		p.next++
	}
	s := anyTagRe.ReplaceAllString(rest[m[2]:m[3]], "")
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}

// child returns the ID of the n'th block inside the block id.
func child(id string, n int) string {
	if id == "" {
		return fmt.Sprintf("d%d", n)
	}
	return fmt.Sprintf("%s.%d", id, n)
}
