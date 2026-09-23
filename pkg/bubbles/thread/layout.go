package thread

import (
	"sort"
	"strings"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"
)

// statusIndent lines the status up with the body, which glamour indents by
// two cells.
const statusIndent = "  "

// renderDoc renders the header and the markdown body at the current width,
// unless they were already rendered at it.
func (m *Model[T]) renderDoc() {
	if !m.hasDoc || m.width <= 0 {
		m.doc, m.docWidth = nil, -1
		return
	}
	if m.docWidth == m.width {
		return
	}
	var lines []string
	if m.header != "" {
		for l := range strings.SplitSeq(m.header, "\n") {
			lines = append(lines, m.fit(l))
		}
		lines = append(lines, m.blank)
	}
	if body := trimBlank(m.renderMarkdown()); len(body) > 0 {
		for _, l := range body {
			lines = append(lines, m.fit(l))
		}
		lines = append(lines, m.blank)
	}
	m.doc, m.docWidth = lines, m.width
}

func (m *Model[T]) renderMarkdown() string {
	if strings.TrimSpace(m.body) == "" {
		return ""
	}
	m.mdRuns++
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(m.markdownStyle()),
		glamour.WithWordWrap(m.width),
	)
	if err != nil {
		return m.body
	}
	out, err := r.Render(m.body)
	if err != nil {
		// Showing the source beats showing nothing.
		return m.body
	}
	return out
}

// renderChunk renders the items of c at the current width, one blank line
// after each.
func (m *Model[T]) renderChunk(c *chunk[T]) {
	lines := make([]string, 0, len(c.items)*4)
	starts := make([]int, 0, len(c.items))
	for _, it := range c.items {
		starts = append(starts, len(lines))
		block := strings.TrimRight(m.render(it, m.width), "\n")
		for l := range strings.SplitSeq(block, "\n") {
			lines = append(lines, m.fit(l))
		}
		lines = append(lines, m.blank)
	}
	c.lines, c.starts, c.height, c.loaded = lines, starts, len(lines), true
}

// fit truncates s to the width and pads it with spaces to exactly the width,
// so View never measures, wraps or pads.
func (m *Model[T]) fit(s string) string {
	s = ansi.Truncate(s, m.width, "…")
	if n := ansi.StringWidth(s); n < m.width {
		s += m.blank[:m.width-n]
	}
	return s
}

// statusText returns the line after the comments, or "" when there is none.
func (m *Model[T]) statusText() string {
	var s string
	switch {
	case !m.hasDoc:
		s = m.spin.View() + " " + m.text.loadingDoc
	case m.tail.loading:
		s = m.spin.View() + " " + m.text.loadingComments
	case m.tail.err != nil:
		s = m.errorText(m.tail.err)
	case m.reloadErr() != nil:
		s = m.errorText(m.reloadErr())
	case m.done() && m.empty():
		s = m.text.empty
	default:
		return ""
	}
	return m.fit(statusIndent + s)
}

// reloadErr returns why reloading a chunk failed. The chunk keeps showing
// what it had, so the error goes in the status.
func (m *Model[T]) reloadErr() error {
	for i := range m.chunks {
		if c := &m.chunks[i]; c.loaded && c.err != nil {
			return c.err
		}
	}
	return nil
}

// done reports whether every chunk is known.
func (m *Model[T]) done() bool {
	return m.started && len(m.chunks) > 0 && m.chunks[len(m.chunks)-1].next == ""
}

func (m *Model[T]) empty() bool {
	for i := range m.chunks {
		if m.chunks[i].height > 0 {
			return false
		}
	}
	return true
}

// layout joins the document, the chunks and the status into lines and
// scrolls back to a.
func (m *Model[T]) layout(a anchor) {
	if m.width <= 0 {
		m.lines, m.statusIdx, m.status = nil, -1, ""
		m.starts = make([]int, len(m.chunks))
		m.vp.SetContentLines(nil)
		return
	}
	n := len(m.doc) + 1
	for i := range m.chunks {
		n += m.chunks[i].height
	}
	lines := make([]string, 0, n)
	lines = append(lines, m.doc...)
	starts := make([]int, len(m.chunks))
	for i := range m.chunks {
		starts[i] = len(lines)
		lines = m.appendChunk(lines, &m.chunks[i])
	}
	m.status, m.statusIdx = m.statusText(), -1
	if m.status != "" {
		m.statusIdx = len(lines)
		lines = append(lines, m.status)
	}
	m.lines, m.starts = lines, starts
	m.vp.SetContentLines(lines)
	m.restore(a)
}

// appendChunk appends the lines of c, or a placeholder of the same height
// if c was evicted, so the lines after it stay where they were.
func (m *Model[T]) appendChunk(lines []string, c *chunk[T]) []string {
	if c.loaded {
		return append(lines, c.lines...)
	}
	if c.height == 0 {
		return lines
	}
	// An evicted chunk on screen is always being fetched again.
	first := m.text.loadingComments
	if c.err != nil {
		first = m.errorText(c.err)
	}
	lines = append(lines, m.fit(statusIndent+first))
	for range c.height - 1 {
		lines = append(lines, m.blank)
	}
	return lines
}

func (m *Model[T]) errorText(err error) string {
	return m.text.errPrefix + m.text.retry + m.styles.Hint.Render(" ("+err.Error()+")")
}

// anchor is a reading position that survives a new layout: a line within a
// comment, a chunk or the document. When the comment's height changes, as
// on a resize, the offset scales with it, so the same part of the same
// comment stays at the top of the screen.
type anchor struct {
	chunk  int // -1 for the document, len(chunks) for the status
	item   int // -1 when the chunk has no rendered items
	off    int
	height int
}

// top is the anchor of a thread scrolled to the top.
var top = anchor{chunk: -1}

func (m *Model[T]) anchor() anchor {
	y := m.vp.YOffset()
	if y <= 0 || len(m.lines) == 0 {
		return top
	}
	if y < len(m.doc) {
		return anchor{chunk: -1, item: -1, off: y, height: len(m.doc)}
	}
	// The last chunk starting at or before y. Empty chunks share a start.
	i := sort.SearchInts(m.starts, y+1) - 1
	for i >= 0 && m.chunks[i].height == 0 {
		i--
	}
	if i < 0 || y >= m.starts[i]+m.chunks[i].height {
		return anchor{chunk: len(m.chunks), item: -1}
	}
	c := &m.chunks[i]
	off := y - m.starts[i]
	if len(c.starts) == 0 {
		return anchor{chunk: i, item: -1, off: off, height: c.height}
	}
	j := sort.SearchInts(c.starts, off+1) - 1
	return anchor{chunk: i, item: j, off: off - c.starts[j], height: itemHeight(c, j)}
}

func (m *Model[T]) restore(a anchor) {
	var y int
	switch {
	case a == top:
	case a.chunk < 0:
		y = scale(a.off, a.height, len(m.doc))
	case a.chunk < len(m.chunks):
		c := &m.chunks[a.chunk]
		y = m.starts[a.chunk]
		if a.item >= 0 && a.item < len(c.starts) {
			y += c.starts[a.item] + scale(a.off, a.height, itemHeight(c, a.item))
		} else {
			y += scale(a.off, a.height, c.height)
		}
	default:
		y = len(m.lines)
	}
	m.vp.SetYOffset(y)
}

func itemHeight[T any](c *chunk[T], j int) int {
	if j+1 < len(c.starts) {
		return c.starts[j+1] - c.starts[j]
	}
	return c.height - c.starts[j]
}

// scale maps a line offset within a block of height from onto a block of
// height to.
func scale(off, from, to int) int {
	if to <= 0 {
		return 0
	}
	if from > 0 && from != to {
		off = off * to / from
	}
	return min(off, to-1)
}

// trimBlank splits s into lines without the blank ones around it.
func trimBlank(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	isBlank := func(l string) bool { return strings.TrimSpace(ansi.Strip(l)) == "" }
	for len(lines) > 0 && isBlank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && isBlank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return lines
}
