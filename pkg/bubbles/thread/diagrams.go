package thread

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/markdown"
)

// folds keeps, by source, the collapsible blocks the reader opened, such
// as diagrams, and the heads that what Markdown rendered showed since the
// thread last took them.
type folds struct {
	open map[string][]int
	seen []seen
}

// seen is what Markdown rendered of src, and its heads.
type seen struct {
	src, text string
	heads     []markdown.Head
}

// with returns open and the blocks of src the reader opened.
func (f *folds) with(src string, open []int) []int {
	o := f.open[src]
	if len(o) == 0 {
		return open
	}
	if len(open) == 0 {
		return o
	}
	return append(slices.Clone(open), o...)
}

// flip opens block of src if it is closed, and closes it if it is open.
func (f *folds) flip(src string, block int) {
	open := slices.Clone(f.open[src])
	if i := slices.Index(open, block); i >= 0 {
		open = slices.Delete(open, i, i+1)
	} else {
		open = append(open, block)
		// In order, so each set renders once.
		slices.Sort(open)
	}
	if f.open == nil {
		f.open = make(map[string][]int)
	}
	if len(open) == 0 {
		delete(f.open, src)
		return
	}
	f.open[src] = open
}

// head is the head of a diagram in the thread: which block of which
// source it is, and where it is and where its code ends.
type head struct {
	src   string
	block int
	// chunk is -1 for the document, and item the comment within it.
	chunk, item int
	line, end   int
	// marked is the head's line with the pointer, or "" if the line has
	// no room for it.
	marked string
}

// OnDiagram reports whether a diagram is on screen for the toggle key to
// open or close: one whose head shows, or else one whose open code does.
func (m Model[T]) OnDiagram() bool {
	return m.target() != nil
}

// target returns the head of the diagram that the toggle key opens or
// closes, or nil.
func (m *Model[T]) target() *head {
	y := m.vp.YOffset()
	end := y + m.height
	var above *head
	for i := range m.heads {
		h := &m.heads[i]
		if h.line >= end {
			break
		}
		if h.line >= y {
			return h
		}
		if h.end > y {
			above = h
		}
	}
	return above
}

// toggle shows or hides the code of the diagram on screen, and keeps its
// head where it was on screen, or at the top if it was above.
func (m *Model[T]) toggle() {
	h := m.target()
	if h == nil {
		return
	}
	t := *h
	row := max(t.line-m.vp.YOffset(), 0)
	m.folds.flip(t.src, t.block)
	if t.chunk < 0 {
		m.docWidth = -1
		m.renderDoc()
	} else if c := &m.chunks[t.chunk]; c.loaded {
		m.renderChunk(c)
	}
	m.layout(m.anchor())
	for _, n := range m.heads {
		if n.chunk == t.chunk && n.item == t.item && n.block == t.block && n.src == t.src {
			m.vp.SetYOffset(n.line - row)
			return
		}
	}
}

// take returns the heads of what Markdown rendered since the last take,
// found in lines, which item holds from line at of its chunk on, and
// forgets them. A render whose lines lines doesn't hold whole and in order
// has no heads.
func (m *Model[T]) take(lines []string, item, at int) []head {
	var out []head
	for _, s := range m.folds.seen {
		o := place(lines, strings.Split(s.text, "\n"))
		if o < 0 {
			continue
		}
		for _, h := range s.heads {
			out = append(out, head{
				src: s.src, block: h.Block, item: item,
				line: at + o + h.Line, end: at + o + h.End,
			})
		}
	}
	m.folds.seen = nil
	return out
}

// place returns where in lines the lines of text start, each at the end
// of a line, as a caller that indents them puts them, or -1.
func place(lines, text []string) int {
	for o := 0; o+len(text) <= len(lines); o++ {
		match := true
		for k, t := range text {
			if !strings.HasSuffix(lines[o+k], t) {
				match = false
				break
			}
		}
		if match {
			return o
		}
	}
	return -1
}

// mark sets the line of each head in lines with the pointer in its first
// cell, if that cell is blank.
func (m *Model[T]) mark(heads []head, lines []string) {
	for i := range heads {
		h := &heads[i]
		l := lines[h.line]
		h.marked = ""
		if strings.HasPrefix(ansi.Strip(l), " ") {
			h.marked = m.text.pointer + ansi.TruncateLeft(l, 1, "")
		}
	}
}
