package diff

import "strings"

// splitLines is the lines of a file's text. A newline ends a line, so the one
// at the end of the text does not start another, and a "\r" before it is
// dropped, as in a patch.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// rangeEnds are the new and old line numbers a hunk starts after and ends at.
// A side with no lines names the line before the hunk, so it ends there.
func rangeEnds(h Hunk) (newBefore, newEnd, oldEnd int) {
	newBefore, newEnd, oldEnd = h.NewStart-1, h.NewStart+h.NewLines-1, h.OldStart+h.OldLines-1
	if h.NewLines == 0 {
		newBefore, newEnd = h.NewStart, h.NewStart
	}
	if h.OldLines == 0 {
		oldEnd = h.OldStart
	}
	return newBefore, newEnd, oldEnd
}

// fullBody is the rows of a file shown whole: the rows of the patch, with the
// lines of content that no hunk covers as context rows before, between and
// after the hunks. The rows of the patch keep their order, and so do the
// hunk headers. A gap's old numbers are its new ones less what the hunks
// before it added.
func fullBody(file int, hunks []Hunk, body []Row, content []string) []Row {
	out := make([]Row, 0, len(body)+len(content))
	next, delta := 1, 0 // the next new line to show, and new numbers less old ones
	gap := func(to int) {
		for ; next <= to && next <= len(content); next++ {
			out = append(out, Row{Kind: KindContext, File: file, Hunk: -1, Old: next - delta, New: next, Text: content[next-1]})
		}
	}
	for _, r := range body {
		if r.Kind == KindHunkHeader && r.Hunk < len(hunks) {
			before, end, oldEnd := rangeEnds(hunks[r.Hunk])
			gap(before)
			next, delta = end+1, end-oldEnd
		}
		out = append(out, r)
	}
	gap(len(content))
	return out
}

// SetFull shows the file at path whole, with content as the text of its new
// side: the lines the hunks leave out come in as context rows, between and
// around them. The hunk headers stay. It reports false, and changes nothing,
// when the file is not in the layout, has no patch to put the lines between,
// was cut short, or did not parse. A file shown whole stays so until
// ClearFull, and setting it again replaces the text.
func (l *Layout) SetFull(path, content string) bool {
	i, ok := l.index[path]
	if !ok {
		return false
	}
	s := &l.files[i]
	if s.lines == 0 || s.file.Truncated {
		return false
	}
	if l.ParseErr(i) != nil {
		return false
	}
	s.full, s.content = true, splitLines(content)
	l.rebuild(i)
	return true
}

// ClearFull shows the file at path as its hunks only again.
func (l *Layout) ClearFull(path string) {
	i, ok := l.index[path]
	if !ok || !l.files[i].full {
		return
	}
	s := &l.files[i]
	s.full, s.content = false, nil
	l.rebuild(i)
}

// Full says whether the file at path is shown whole.
func (l *Layout) Full(path string) bool {
	i, ok := l.index[path]
	return ok && l.files[i].full
}

// widest is the largest line number, on either side, of the rows of file f:
// that of its hunks, and for a file shown whole also that of the lines after
// the last hunk, whose old numbers are their new ones less what the hunks added.
func (l *Layout) widest(f int) int {
	hunks := l.Hunks(f)
	w := 0
	for _, h := range hunks {
		w = max(w, h.OldStart+h.OldLines, h.NewStart+h.NewLines)
	}
	s := &l.files[f]
	if !s.full {
		return w
	}
	n, delta := len(s.content), 0
	if len(hunks) > 0 {
		_, end, oldEnd := rangeEnds(hunks[len(hunks)-1])
		delta = end - oldEnd
	}
	return max(w, n, n-delta)
}

// rebuild parses file i again, and moves the rows after it.
func (l *Layout) rebuild(i int) {
	s := &l.files[i]
	s.parsed, s.body, s.hunks = false, nil, nil
	l.ensure(i)
	l.resum(i)
}

// SetFull shows the file at path whole; see [Layout.SetFull]. The cursor stays
// on its line, on the row of the window it was on, and the rows after the file
// keep their place on the screen. It reports whether the file is shown whole.
func (m *Model) SetFull(path, content string) bool {
	return m.reflow(path, func() bool { return m.layout.SetFull(path, content) }, false)
}

// ClearFull shows the file at path as its hunks only again. A cursor on a
// line of no hunk goes to the nearest line of one, above it first.
func (m *Model) ClearFull(path string) {
	m.reflow(path, func() bool { m.layout.ClearFull(path); return true }, true)
}

// Full says whether the file at path is shown whole.
func (m Model) Full(path string) bool { return m.layout.Full(path) }

// reflow runs change, which moves the rows of the file at path, and keeps the
// cursor on what it was on: the same line, or the same hunk header, at the same
// row of the window. It reports what change did.
func (m *Model) reflow(path string, change func() bool, nearest bool) bool {
	f := m.layout.FileIndex(path)
	if f < 0 {
		return false
	}
	start, _ := m.layout.FileRow(f)
	end, _ := m.layout.FileRow(f + 1)
	offset := m.cursor - m.top
	inside := m.cursor >= start && m.cursor < end
	var (
		pos     Pos
		onLine  bool
		hunk    = -1
		onFile  = inside && m.cursor == start
		blocked bool
	)
	if inside {
		pos, onLine = m.layout.PosAt(m.cursor)
		if r, _ := m.layout.RowAt(m.cursor); r.Kind == KindHunkHeader {
			hunk = r.Hunk
		} else if r.Kind == KindBlock {
			blocked = true
		}
		if onLine && nearest {
			pos = m.layout.nearestHunkLine(f, m.cursor, pos)
		}
	}
	if !change() {
		return false
	}
	// The tokens of the rows as they were no longer fit. A read in flight for
	// another file would be dropped too, so every file is lexed again.
	m.hl.gen++
	clear(m.hl.asked)
	clear(m.hl.spans)
	newEnd, _ := m.layout.FileRow(f + 1)
	switch {
	case inside:
		row := m.cursor
		switch {
		case onFile:
			row = start
		case onLine:
			row, _ = m.layout.Find(pos)
		case hunk >= 0:
			row = m.layout.hunkRow(f, hunk, start)
		case blocked:
			row = min(row, newEnd-1)
		}
		m.cursor = row
		m.top = max(row-offset, 0)
	case m.cursor >= end:
		m.cursor += newEnd - end
		if m.top >= end {
			m.top += newEnd - end
		}
	}
	m.rebuildSearch()
	m.scroll()
	m.dirty = true
	return true
}

// hunkRow is the row of the header of hunk h of file f, which starts at row
// start, or start when the file has no such hunk.
func (l *Layout) hunkRow(f, h, start int) int {
	end, _ := l.FileRow(f + 1)
	for r := start + 1; r < end; r++ {
		if row, ok := l.RowAt(r); ok && row.Kind == KindHunkHeader && row.Hunk == h {
			return r
		}
	}
	return start
}

// nearestHunkLine is the position of the line of a hunk nearest to row, a
// line of file f the position p names, which is p itself if a hunk has it.
// Between two the one above wins.
func (l *Layout) nearestHunkLine(f, row int, p Pos) Pos {
	start, _ := l.FileRow(f)
	s := &l.files[f]
	at := l.bodyIndex(f, row-start-1)
	if at < 0 || at >= len(s.body) || s.body[at].Hunk >= 0 {
		return p
	}
	in := func(i int) (Pos, bool) {
		if i < 0 || i >= len(s.body) || s.body[i].Hunk < 0 {
			return Pos{}, false
		}
		switch r := &s.body[i]; r.Kind {
		case KindContext, KindAdded:
			return Pos{Path: s.file.Path, Side: NewSide, Line: r.New}, true
		case KindDeleted:
			return Pos{Path: s.file.Path, Side: OldSide, Line: r.Old}, true
		case KindFileHeader, KindHunkHeader, KindNoNewline, KindNote, KindRaw, KindBlock:
		}
		return Pos{}, false
	}
	for d := 1; d < len(s.body); d++ {
		if q, ok := in(at - d); ok {
			return q
		}
		if q, ok := in(at + d); ok {
			return q
		}
	}
	return p
}
