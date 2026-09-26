package cmdline

import (
	"slices"
	"unicode/utf8"
)

// Candidate is one way to complete the line.
type Candidate struct {
	// Text replaces the span of the line from Start to End when the
	// candidate is chosen.
	Text string
	// Label is what the candidate row shows, such as the last part of a
	// path. It defaults to Text.
	Label string
	// Detail is shown dimmed after the label, such as what a command does.
	Detail string
	// Start and End are the byte offsets in the line of the span that Text
	// replaces, such as the word under the cursor. They must fall on rune
	// boundaries, with Start <= End <= len(line); a candidate whose span
	// doesn't is dropped.
	Start, End int
}

// Complete returns the candidates that complete line, where cursor is the
// byte offset of the cursor in line. The command line calls it while
// focused: in Update on every change to the line or move of the cursor
// (but not while tab and shift+tab cycle through the candidates), and in
// Open, Focus, SetValue and SetComplete. It must be fast and do no I/O:
// complete from data already in memory. The command line copies what it
// returns.
type Complete func(line string, cursor int) []Candidate

// completion is the state of the candidate row.
type completion struct {
	cands []Candidate
	// items are the candidates rendered, in the order of cands.
	items []item
	// sel is the candidate inserted in the line, or -1 while the line is
	// as typed.
	sel int
	// first is the first candidate the row shows.
	first int
	// line and cursor are the line as typed before the user cycled, which
	// the spans of cands refer to.
	line   string
	cursor int
	// row caches the rendered row for rowWidth, rowSel and rowFirst.
	row                        string
	rowWidth, rowSel, rowFirst int
}

// item is a candidate rendered, plain and selected, and its width.
type item struct {
	plain, selected string
	width           int
}

// refresh asks for the candidates of the line as it is, which ends any
// cycling.
func (m *Model) refresh() {
	c := &m.comp
	c.sel, c.first = -1, 0
	line := m.input.Value()
	c.line, c.cursor = line, byteOffset(line, m.input.Position())
	var cands []Candidate
	if m.complete != nil && m.focused {
		cands = m.complete(line, c.cursor)
	}
	m.setCandidates(cands)
}

// setCandidates keeps the valid candidates in cands, and renders them
// unless they look the same as those shown, as they do while the user
// types on in a word.
func (m *Model) setCandidates(cands []Candidate) {
	c := &m.comp
	var valid []Candidate
	for _, cd := range cands {
		if spanOK(c.line, cd.Start, cd.End) {
			valid = append(valid, cd)
		}
	}
	same := slices.EqualFunc(valid, c.cands, func(a, b Candidate) bool {
		return a.Text == b.Text && a.Label == b.Label && a.Detail == b.Detail
	})
	c.cands = valid
	if !same {
		m.renderItems()
	}
}

// spanOK reports whether start and end are rune boundaries of line in
// order.
func spanOK(line string, start, end int) bool {
	if start < 0 || start > end || end > len(line) {
		return false
	}
	return boundary(line, start) && boundary(line, end)
}

func boundary(s string, i int) bool {
	return i == len(s) || utf8.RuneStart(s[i])
}

// cycle inserts the candidate by steps of d after the selected one. Past
// either end, it puts back the line as typed, as vim does.
func (m *Model) cycle(d int) {
	c := &m.comp
	n := len(c.cands)
	if n == 0 {
		return
	}
	// The line as typed sits between the last candidate and the first.
	c.sel = (c.sel+1+d+n+1)%(n+1) - 1
	line, cursor := c.line, c.cursor
	if c.sel >= 0 {
		cd := c.cands[c.sel]
		line = c.line[:cd.Start] + cd.Text + c.line[cd.End:]
		cursor = cd.Start + len(cd.Text)
	}
	m.input.SetValue(line)
	m.input.SetCursor(utf8.RuneCountInString(line[:cursor]))
	m.scroll()
}

// scroll moves the row so it shows the selected candidate.
func (m *Model) scroll() {
	c := &m.comp
	switch {
	case c.sel < 0:
		c.first = 0
	case c.sel < c.first:
		c.first = c.sel
	default:
		for c.first < c.sel {
			if end, _ := m.fitRow(c.first); c.sel < end {
				break
			}
			c.first++
		}
	}
}

// widen moves the row back while the selected candidate stays on it, so a
// row that grew wider shows as many candidates as it can.
func (m *Model) widen() {
	c := &m.comp
	for c.sel >= 0 && c.first > 0 {
		if end, _ := m.fitRow(c.first - 1); c.sel >= end {
			return
		}
		c.first--
	}
}

// byteOffset returns the byte offset of rune pos in s.
func byteOffset(s string, pos int) int {
	for i := range s {
		if pos == 0 {
			return i
		}
		pos--
	}
	return len(s)
}
