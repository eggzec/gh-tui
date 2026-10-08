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
		return Pos{Path: l.files[r.File].file.Path, Side: New, Line: r.New}, true
	case KindDeleted:
		return Pos{Path: l.files[r.File].file.Path, Side: Old, Line: r.Old}, true
	case KindFileHeader, KindHunkHeader, KindNoNewline, KindNote, KindRaw:
	}
	return Pos{}, false
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
	for r := range s.body {
		b := &s.body[r]
		switch b.Kind {
		case KindContext:
			if (p.Side == New && b.New == p.Line) || (p.Side == Old && b.Old == p.Line) {
				return row + 1 + r, true
			}
		case KindAdded:
			if p.Side == New && b.New == p.Line {
				return row + 1 + r, true
			}
		case KindDeleted:
			if p.Side == Old && b.Old == p.Line {
				return row + 1 + r, true
			}
		case KindFileHeader, KindHunkHeader, KindNoNewline, KindNote, KindRaw:
		}
	}
	return row, false
}
