package diff

// PosAt is the position row i stands for: a line of the new side for an
// added or context line, a line of the old side for a deleted one. Headers,
// markers and notes have none, so it reports false for them and for a row
// out of range.
func (l *Layout) PosAt(i int) (Pos, bool) {
	if i < 0 || i >= l.Len() {
		return Pos{}, false
	}
	r, _ := l.RowAt(i)
	switch r.Kind {
	case KindContext, KindAdded:
		return Pos{Path: l.files[r.File].file.Path, Side: NewSide, Line: r.New}, true
	case KindDeleted:
		return Pos{Path: l.files[r.File].file.Path, Side: OldSide, Line: r.Old}, true
	case KindFileHeader, KindHunkHeader, KindNoNewline, KindNote, KindRaw, KindBlock:
	}
	return Pos{}, false
}

// OnHeader reports whether the cursor is on the header of a file.
func (m Model) OnHeader() bool {
	r, ok := m.layout.RowAt(m.cursor)
	return ok && r.Kind == KindFileHeader
}

// SetAnchor starts a selection on the line p, which reaches to the cursor and
// follows it. ClearAnchor ends it.
func (m *Model) SetAnchor(p Pos) { m.anchor = &p }

// ClearAnchor ends the selection.
func (m *Model) ClearAnchor() { m.anchor = nil }

// Anchor returns the line a selection started on.
func (m Model) Anchor() (Pos, bool) {
	if m.anchor == nil {
		return Pos{}, false
	}
	return *m.anchor, true
}

// selection returns the first and last row of the selection, or an empty
// range when there is none or its line is not shown.
func (m Model) selection() (lo, hi int) {
	if m.anchor == nil {
		return 0, -1
	}
	row, found := m.layout.Find(*m.anchor)
	if !found {
		return 0, -1
	}
	return min(row, m.cursor), max(row, m.cursor)
}

// Line returns the text of the line p names, as the fetched files show it,
// and whether the diff shows that line. known is false when that can't be
// said yet: the file may still come with a later page, or the diff of the file
// is cut short, so that the line may be in the part that isn't there.
func (m Model) Line(p Pos) (text string, shown, known bool) {
	i := m.layout.FileIndex(p.Path)
	if i < 0 {
		return "", false, m.pg.done
	}
	l := m.layout
	l.ensure(i)
	if r, ok := lineIn(l.files[i].body, p); ok {
		return l.files[i].body[r].Text, true, true
	}
	f := l.files[i].file
	return "", false, !f.Truncated && f.Patch != ""
}

// Find returns the row of a position, parsing the file. When the position is
// not a line the diff shows, found is false and row is the file's header, or
// -1 if the file is not in the layout. A collapsed file is not found: expand
// it and ask again. A context line is found on either side. Find scans the
// file's rows one by one, so a widget that places many comments may want an
// index of its own.
func (l *Layout) Find(p Pos) (row int, found bool) {
	f, ok := l.index[p.Path]
	if !ok {
		return -1, false
	}
	row = l.starts[f]
	s := &l.files[f]
	if s.collapsed || p.Line < 1 {
		return row, false
	}
	l.ensure(f)
	if r, ok := lineIn(s.body, p); ok {
		// The rows of blocks above the line push it down.
		return row + 1 + r + s.shift(r), true
	}
	return row, false
}

// SameHunk reports whether the lines a and b are in one hunk of one file: both
// are shown, and no hunk header lies between them.
func (l *Layout) SameHunk(a, b Pos) bool {
	if a.Path != b.Path {
		return false
	}
	ra, okA := l.Find(a)
	rb, okB := l.Find(b)
	if !okA || !okB {
		return false
	}
	for i := min(ra, rb); i <= max(ra, rb); i++ {
		if r, ok := l.RowAt(i); ok && r.Kind == KindHunkHeader {
			return false
		}
	}
	return true
}

// SameHunk reports whether the lines a and b are in one hunk, as
// Layout.SameHunk does.
func (m Model) SameHunk(a, b Pos) bool { return m.layout.SameHunk(a, b) }

// lineIn is the index of the row of body that shows p, which is a line of
// some file.
func lineIn(body []Row, p Pos) (int, bool) {
	for r := range body {
		b := &body[r]
		switch b.Kind {
		case KindContext:
			if (p.Side == NewSide && b.New == p.Line) || (p.Side == OldSide && b.Old == p.Line) {
				return r, true
			}
		case KindAdded:
			if p.Side == NewSide && b.New == p.Line {
				return r, true
			}
		case KindDeleted:
			if p.Side == OldSide && b.Old == p.Line {
				return r, true
			}
		case KindFileHeader, KindHunkHeader, KindNoNewline, KindNote, KindRaw, KindBlock:
		}
	}
	return 0, false
}
