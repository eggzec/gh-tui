package diff

import tea "charm.land/bubbletea/v2"

// SetHeaderMark sets the function whose string is drawn at the end of each
// file's header row; see [WithHeaderMark]. Nil draws nothing.
func (m *Model) SetHeaderMark(mark func(File) string) { m.headerMark = mark }

// SetLineMark sets the function whose string is drawn in the second cell of the
// gutter of each added, deleted and context line, such as a mark for a line that
// has a comment. It gets the position of the line on the side it is drawn for,
// and for a context line the new side first, then the old; an empty string
// draws nothing. It runs on every draw of a line, so it must be cheap and its
// string one cell wide. Nil draws nothing.
func (m *Model) SetLineMark(mark func(Pos) string) { m.lineMark = mark }

// markOf is the mark of the line row is, or "" when the row is no line or the
// line has none.
func (m Model) markOf(row Row) string {
	if m.lineMark == nil || row.File < 0 || row.File >= len(m.layout.files) {
		return ""
	}
	path := m.layout.files[row.File].file.Path
	switch row.Kind {
	case KindAdded:
		return m.lineMark(Pos{Path: path, Side: NewSide, Line: row.New})
	case KindDeleted:
		return m.lineMark(Pos{Path: path, Side: OldSide, Line: row.Old})
	case KindContext:
		if s := m.lineMark(Pos{Path: path, Side: NewSide, Line: row.New}); s != "" {
			return s
		}
		return m.lineMark(Pos{Path: path, Side: OldSide, Line: row.Old})
	case KindFileHeader, KindHunkHeader, KindNoNewline, KindNote, KindRaw, KindBlock:
	}
	return ""
}

// SetStatusNote sets the function whose string ends the status line, after the
// position, such as a count of the files that have been read. It runs on every
// draw of the line, so it must be cheap. Nil, or an empty string, adds nothing.
func (m *Model) SetStatusNote(note func() string) { m.statusNote = note }

// SetFirstFilesNote ends the diff with a note row saying that it shows only the
// first n files, or takes the row away with zero; see [WithFirstFilesNote]. The
// row comes or goes at once, with the cursor and the window on the rows they
// were on.
func (m *Model) SetFirstFilesNote(n int) {
	n = max(n, 0)
	if m.layout.firstFiles == n {
		return
	}
	h := m.holdRows()
	m.layout.firstFiles = n
	m.restore(h)
	m.dirty = true
}

// Collapsed says whether the file at path shows only its header. It is false
// for a path that has not been fetched.
func (m Model) Collapsed(path string) bool {
	return m.layout.Collapsed(m.layout.FileIndex(path))
}

// SetCollapsed collapses the file at path to its header, or expands it, as
// the fold key does on the header, and does nothing for a path that has not
// been fetched. The cursor and the window stay on the rows they were on; a
// cursor inside a file that collapses goes to its header.
func (m *Model) SetCollapsed(path string, collapsed bool) {
	i := m.layout.FileIndex(path)
	if i < 0 || m.layout.Collapsed(i) == collapsed {
		return
	}
	h := m.holdRows()
	m.layout.SetCollapsed(i, collapsed)
	m.restore(h)
	m.dirty = true
}

// FileEndVisible reports whether the last row of the file the cursor is in is
// in the window, so that reading on would go past it. It is false with no
// files.
func (m Model) FileEndVisible() bool {
	f, ok := m.layout.FileAt(m.cursor)
	if !ok {
		return false
	}
	next, _ := m.layout.FileRow(f + 1)
	return next <= m.top+m.bodyHeight()
}

// PageDown moves the cursor down a window, as the page down key does, and
// returns the command that fetches what the window then shows.
func (m *Model) PageDown() tea.Cmd {
	m.cancelPending()
	m.cursor += m.bodyHeight()
	return m.sync()
}

// PageDownWithinFile is [Model.PageDown] that stops at the last row of the
// file the cursor is in when a whole page would cross into the next file while
// the file's end is not yet in the window, so that the end shows with the cursor
// still in the file. Once the end shows, it pages on as PageDown does.
func (m *Model) PageDownWithinFile() tea.Cmd {
	if f, ok := m.layout.FileAt(m.cursor); ok && !m.FileEndVisible() {
		next, _ := m.layout.FileRow(f + 1)
		if m.cursor+m.bodyHeight() >= next {
			m.cancelPending()
			m.cursor = next - 1
			return m.sync()
		}
	}
	return m.PageDown()
}

// cancelPending drops a seek waiting for pages and a search waiting for a
// match, which a move of the cursor overrides.
func (m *Model) cancelPending() {
	m.pg.seek = nil
	m.search.waiting = false
}
