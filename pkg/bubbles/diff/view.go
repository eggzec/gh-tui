package diff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// twoNumbersFrom is the width from which the gutter shows the old and the
// new line number; below it, only one.
const twoNumbersFrom = 70

// minNumberWidth is the least width of a line number in the gutter.
const minNumberWidth = 3

// visible is one row of the window.
type visible struct {
	row    Row
	index  int  // the row's number; -1 for none
	status bool // the loading, error or empty row
	sticky bool // the header that stands in for the file's rows above
}

// View renders the visible rows in exactly Height lines of Width cells.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	h := m.bodyHeight()
	n := m.layout.Len()
	rows := make([]visible, h)
	sticky := m.sticky()
	for s := range rows {
		r := m.top + s
		switch {
		case s == 0 && sticky:
			f, _ := m.layout.FileAt(m.top)
			start, _ := m.layout.FileRow(f)
			row, _ := m.layout.RowAt(start)
			rows[s] = visible{row: row, index: start, sticky: true}
		case r < n:
			row, _ := m.layout.RowAt(r)
			rows[s] = visible{row: row, index: r}
		case r == n && m.hasStatus():
			rows[s] = visible{index: -1, status: true}
		default:
			rows[s] = visible{index: -1}
		}
	}
	numW := m.numWidth()

	var b strings.Builder
	b.Grow(m.height * (m.width + 32))
	for s, v := range rows {
		if s > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.line(v, numW))
	}
	if m.height >= 3 {
		b.WriteByte('\n')
		b.WriteString(m.statusLine())
	}
	return b.String()
}

// statusLine renders the line under the rows: the search input while it is
// open, and otherwise where the cursor is, and what the search shown found.
func (m Model) statusLine() string {
	if m.searching {
		return fit(m.input.View(), m.width)
	}
	text, none := m.searchStatus()
	if text == "" {
		return fit(m.wrap.Status.on(fit(m.Status(), m.width)), m.width)
	}
	sep, style := "", m.wrap.Status
	if m.Status() != "" {
		sep = m.styles.Separator
	}
	if none {
		style = m.wrap.Error
	}
	return fit(m.wrap.Status.on(m.Status()+sep)+style.on(termtext.OneLine(text)), m.width)
}

// Status says where the cursor is, such as "hunk 2/5 · file 3/10": the
// hunk among those of its file, left out on a header or a note, and the
// file among those fetched, with a "+" while more are to come. It is empty
// while the cursor is in no file.
func (m Model) Status() string {
	f, ok := m.layout.FileAt(m.cursor)
	if !ok {
		return ""
	}
	var parts []string
	if row, _ := m.layout.RowAt(m.cursor); row.Hunk >= 0 {
		parts = append(parts, fmt.Sprintf("hunk %d/%d", row.Hunk+1, len(m.layout.Hunks(f))))
	}
	more := ""
	if !m.pg.done {
		more = "+"
	}
	parts = append(parts, fmt.Sprintf("file %d/%d%s", f+1, m.layout.Files(), more))
	return strings.Join(parts, m.styles.Separator)
}

// numWidth is the width of a line number in the gutter: that of the widest
// line number of the files in the window, from their hunks, so that it does
// not change while the window scrolls inside a file. Folded files show no
// numbers.
func (m Model) numWidth() int {
	n := m.layout.Len()
	if n == 0 || m.top >= n {
		return minNumberWidth
	}
	first, ok := m.layout.FileAt(m.top)
	if !ok {
		return minNumberWidth
	}
	last, ok := m.layout.FileAt(min(m.top+m.bodyHeight()-1, n-1))
	if !ok {
		last = m.layout.Files() - 1
	}
	widest := 0
	for f := first; f <= last; f++ {
		if m.layout.Collapsed(f) {
			continue
		}
		for _, h := range m.layout.Hunks(f) {
			widest = max(widest, h.OldStart+h.OldLines, h.NewStart+h.NewLines)
		}
	}
	return max(len(strconv.Itoa(widest)), minNumberWidth)
}

// textWidth is the width of the text of a line, when its gutter is numW
// wide.
func (m Model) textWidth(numW int) int {
	return max(m.width-gutterWidth-m.numberCells(numW)-1, 0)
}

// numberCells is the width of the line numbers of the gutter.
func (m Model) numberCells(numW int) int {
	if m.width >= twoNumbersFrom {
		return 2*numW + 2
	}
	return numW + 1
}

// line renders one row of the window.
func (m Model) line(v visible, numW int) string {
	if v.index < 0 && !v.status {
		return strings.Repeat(" ", m.width)
	}
	gutter := "  "
	switch {
	case v.index == m.cursor && !v.sticky && m.focused:
		gutter = m.gutterFocused
	case v.index == m.cursor && !v.sticky:
		gutter = m.gutterBlurred
	}
	room := max(m.width-gutterWidth, 0)
	if v.status {
		return fit(gutter+m.statusRow(room), m.width)
	}
	row := v.row
	switch row.Kind {
	case KindFileHeader:
		return fit(gutter+m.fileHeader(row.File, room), m.width)
	case KindHunkHeader:
		return fit(gutter+m.wrap.HunkHeader.on(fit(termtext.OneLine(row.Text), room)), m.width)
	case KindNote:
		blank := strings.Repeat(" ", min(m.numberCells(numW), room))
		return fit(gutter+m.wrap.Note.on(fit(blank+termtext.OneLine(row.Text), room)), m.width)
	case KindContext, KindAdded, KindDeleted, KindNoNewline, KindRaw:
	}
	style, marker := m.wrap.Context, " "
	switch row.Kind {
	case KindAdded:
		style, marker = m.wrap.Added, "+"
	case KindDeleted:
		style, marker = m.wrap.Deleted, "-"
	case KindNoNewline:
		style = m.wrap.NoNewline
	case KindRaw:
		marker = ""
	case KindFileHeader, KindHunkHeader, KindContext, KindNote:
	}
	numbers := m.numbers(row, numW)
	width := max(m.textWidth(numW)+1-len(marker), 0)
	hits := m.rowHits(row, v.index)
	spans := m.rowSpans(row, v.index)
	if spans == nil && hits != nil {
		// The line shows plain, in the style of its marker.
		return fit(gutter+m.wrap.LineNumber.on(numbers)+style.on(marker)+m.coloured(row, nil, tokenStyles{text: style}, width, hits), m.width)
	}
	if spans != nil {
		code := m.wrap.code[codeContext]
		switch row.Kind {
		case KindAdded:
			code = m.wrap.code[codeAdded]
		case KindDeleted:
			code = m.wrap.code[codeDeleted]
		case KindFileHeader, KindHunkHeader, KindContext, KindNoNewline, KindNote, KindRaw:
		}
		return fit(gutter+m.wrap.LineNumber.on(numbers)+style.on(marker)+m.coloured(row, spans, code, width, hits), m.width)
	}
	return fit(gutter+m.wrap.LineNumber.on(numbers)+style.on(marker+m.text(row, width)), m.width)
}

// numbers is the line numbers of the gutter for a line: the old and the
// new one, or, in a narrow view, the new one, or the old of a deleted line.
func (m Model) numbers(row Row, numW int) string {
	if row.Kind == KindNoNewline || row.Kind == KindRaw {
		return strings.Repeat(" ", m.numberCells(numW))
	}
	if m.width >= twoNumbersFrom {
		return padNumber(row.Old, numW) + " " + padNumber(row.New, numW) + " "
	}
	if row.New > 0 {
		return padNumber(row.New, numW) + " "
	}
	return padNumber(row.Old, numW) + " "
}

// padNumber is n right-aligned in w cells, or blank for 0.
func padNumber(n, w int) string {
	if n == 0 {
		return strings.Repeat(" ", w)
	}
	s := strconv.Itoa(n)
	if len(s) < w {
		return strings.Repeat(" ", w-len(s)) + s
	}
	return s
}

// text is the text of a line, cleaned and scrolled sideways, padded to
// width cells.
func (m Model) text(row Row, width int) string {
	s := termtext.Clean(row.Text, m.tabs)
	if m.left > 0 || ansi.StringWidth(s) > width {
		s = ansi.Cut(s, m.left, m.left+width)
	}
	return fit(s, width)
}

// coloured is the text of a line in the colors of its spans, cleaned and
// scrolled sideways like [Model.text], and padded to width cells in the
// style of the line, with the matches of a search in hits over them.
func (m Model) coloured(row Row, spans []syntax.Span, code tokenStyles, width int, hits []hit) string {
	s := termtext.Clean(row.Text, m.tabs)
	sw := ansi.StringWidth(s)
	out := code.paint(s, spans, hits, m.wrap.match, m.wrap.current)
	if m.left > 0 || sw > width {
		out = ansi.Cut(out, m.left, m.left+width)
		sw = ansi.StringWidth(out)
	}
	if sw < width {
		out += code.text.on(strings.Repeat(" ", width-sw))
	}
	return out
}

// fileHeader renders the header of file f in room cells: a fold glyph, the
// path, and the counts of added and deleted lines.
func (m Model) fileHeader(f, room int) string {
	st := &m.styles
	file, _ := m.layout.File(f)
	glyph := st.FoldOpen
	if m.layout.Collapsed(f) {
		glyph = st.FoldClosed
	}
	name := termtext.OneLine(file.Path)
	if file.OldPath != "" && file.OldPath != file.Path {
		name = termtext.OneLine(file.OldPath) + " " + st.RenameArrow + " " + name
	}
	counts := ""
	if file.Additions+file.Deletions > 0 {
		counts = "+" + strconv.Itoa(file.Additions) + " -" + strconv.Itoa(file.Deletions)
	}
	left := room - 2
	if counts != "" {
		left -= len(counts) + 1
	}
	name = termtext.Truncate(name, max(left, 1), st.Ellipsis)
	out := m.wrap.FileHeader.on(termtext.Cells(glyph, 1) + " " + name)
	if counts != "" {
		out += " " + m.wrap.Added.on("+"+strconv.Itoa(file.Additions)) + " " + m.wrap.Deleted.on("-"+strconv.Itoa(file.Deletions))
	}
	return out
}

// statusRow renders the row that follows the rows: loading, the error with
// what to do about it, or that there are no files.
func (m Model) statusRow(room int) string {
	st := &m.styles
	switch {
	case m.pg.err != nil:
		msg, _, _ := strings.Cut(m.pg.err.Error(), "\n")
		hint := ""
		if k := m.keyMap.Retry.Help().Key; k != "" {
			hint = st.Separator + k + " to retry"
		}
		text := termtext.Truncate(st.ErrorGlyph+" Couldn't load files: "+termtext.OneLine(msg), max(room-ansi.StringWidth(hint), 0), st.Ellipsis)
		return m.wrap.Error.on(text) + m.wrap.Hint.on(hint)
	case m.pg.fetching:
		return m.wrap.Loading.on("Loading" + st.Ellipsis)
	}
	return m.wrap.Empty.on("No files changed.")
}

// fit cuts or pads s, which may hold styles, to exactly w cells.
func fit(s string, w int) string {
	switch sw := ansi.StringWidth(s); {
	case sw > w:
		return termtext.Truncate(s, w, "")
	case sw < w:
		return s + strings.Repeat(" ", w-sw)
	}
	return s
}
