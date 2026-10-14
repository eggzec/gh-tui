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
