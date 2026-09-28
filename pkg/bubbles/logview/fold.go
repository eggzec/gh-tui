package logview

import "slices"

// rebuild lists the rows shown, skipping the insides of collapsed folds.
// It makes a new slice, so copies of the model keep theirs.
func (m *Model) rebuild() {
	vis := make([]int, 0, len(m.vis))
	for r := 0; r < len(m.rows); {
		vis = append(vis, r)
		if f := m.rows[r].fold; f >= 0 && !m.folds[f].open {
			r = m.folds[f].end
			continue
		}
		r++
	}
	m.vis = vis
}

// locate returns the entry of vis that shows row r, or the header of the
// collapsed fold that hides it.
func (m *Model) locate(r int) int {
	for {
		if v, ok := slices.BinarySearch(m.vis, r); ok {
			return v
		}
		f := m.rows[r].parent
		if f < 0 {
			// Rows outside folds are always shown.
			v, _ := slices.BinarySearch(m.vis, r)
			return min(v, len(m.vis)-1)
		}
		r = m.folds[f].head
	}
}

// cursorRow returns the row under the cursor, or -1 in an empty log.
func (m *Model) cursorRow() int {
	if len(m.vis) == 0 {
		return -1
	}
	return m.vis[m.cur]
}

// refold applies a change to the folds, on a copy of them, and keeps the
// cursor and the window on the rows they were on, or on the headers that
// now hide them.
func (m *Model) refold(change func(folds []fold)) {
	if len(m.vis) == 0 {
		return
	}
	cur, top := m.vis[m.cur], m.vis[m.top]
	m.folds = slices.Clone(m.folds)
	change(m.folds)
	m.rebuild()
	m.cur, m.top = m.locate(cur), m.locate(top)
	if m.vis[m.top] != top {
		m.row = 0
	}
	m.clamp()
	m.show()
}

// toggle expands or collapses the fold under the cursor. On a row inside a
// fold, it collapses that fold.
func (m *Model) toggle() {
	r := m.cursorRow()
	if r < 0 {
		return
	}
	if f := m.rows[r].fold; f >= 0 {
		m.setOpen(f, !m.folds[f].open)
		return
	}
	m.collapse()
}

// expand expands the fold under the cursor.
func (m *Model) expand() {
	if r := m.cursorRow(); r >= 0 && m.rows[r].fold >= 0 {
		m.setOpen(m.rows[r].fold, true)
	}
}

// collapse collapses the fold under the cursor if it is expanded, and the
// fold the cursor is in otherwise, moving the cursor to its header.
func (m *Model) collapse() {
	r := m.cursorRow()
	if r < 0 {
		return
	}
	f := m.rows[r].fold
	if f < 0 || !m.folds[f].open {
		f = m.rows[r].parent
	}
	if f >= 0 {
		m.setOpen(f, false)
	}
}

func (m *Model) setOpen(f int, open bool) {
	if m.folds[f].open == open {
		return
	}
	m.refold(func(folds []fold) { folds[f].open = open })
}

// ExpandAll expands every section and group.
func (m *Model) ExpandAll() { m.setAll(true) }

// CollapseAll collapses every section and group.
func (m *Model) CollapseAll() { m.setAll(false) }

// FoldAll collapses every section when any is expanded, and expands every
// section otherwise. Groups keep their state, and the cursor stays on its
// row or on the section that now hides it.
func (m *Model) FoldAll() {
	open := !slices.ContainsFunc(m.folds, func(f fold) bool { return f.sec >= 0 && f.open })
	m.refold(func(folds []fold) {
		for i := range folds {
			if folds[i].sec >= 0 {
				folds[i].open = open
			}
		}
	})
}

func (m *Model) setAll(open bool) {
	m.refold(func(folds []fold) {
		for i := range folds {
			folds[i].open = open
		}
	})
}

// reveal expands the folds that hide row r and moves the cursor to it,
// scrolling it into view with some of what comes before it.
func (m *Model) reveal(r int) {
	hidden := false
	for f := m.rows[r].parent; f >= 0; f = m.folds[f].parent {
		hidden = hidden || !m.folds[f].open
	}
	if hidden {
		m.refold(func(folds []fold) {
			for f := m.rows[r].parent; f >= 0; f = folds[f].parent {
				folds[f].open = true
			}
		})
	}
	m.jumpTo(m.locate(r))
}

// FocusFailed collapses every section but the first that failed, and moves
// the cursor to its first error, expanding the group it is in. Without
// failed sections, it moves to the first error of the log. It reports
// whether there was anything to focus.
func (m *Model) FocusFailed() bool {
	sec := slices.IndexFunc(m.secs, func(s section) bool { return s.failed })
	if sec < 0 {
		if len(m.errs) == 0 {
			return false
		}
		m.goToIssue(Error, 0)
		return true
	}
	m.refold(func(folds []fold) {
		for i := range folds {
			if folds[i].sec >= 0 {
				folds[i].open = folds[i].sec == sec
			}
		}
	})
	f := m.folds[m.secs[sec].fold]
	if i, _ := slices.BinarySearch(m.errs, f.head); i < len(m.errs) && m.errs[i] < f.end {
		m.goToIssue(Error, i)
		return true
	}
	m.reveal(f.head)
	return true
}
